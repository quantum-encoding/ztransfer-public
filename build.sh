#!/bin/bash
set -e

# ztransfer cross-platform build script
# Usage:
#   ./build.sh                          # Build CLI + GUI for current platform
#   ./build.sh --target macos-arm64     # Build for specific target
#   ./build.sh --target linux-amd64     # Cross-compile for Linux
#   ./build.sh --target all             # Build all targets
#   ./build.sh --cli-only               # Skip GUI build
#   ./build.sh --gui-only               # Skip CLI build

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
ZIG="${ZIG:-/usr/local/zig/zig}"
ZIG_CRYPTO="${ZIG_CRYPTO:-$PROJECT_DIR/../quantum-zig-forge/programs/zig-quantum-encryption}"
OUTPUT_DIR="$PROJECT_DIR/dist"
VERSION="0.1.0"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}==>${NC} ${BOLD}$1${NC}"; }
ok()    { echo -e "${GREEN}  ✓${NC} $1"; }
fail()  { echo -e "${RED}  ✗${NC} $1"; exit 1; }

# Parse args
TARGET=""
CLI_ONLY=false
GUI_ONLY=false
RELEASE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --target)   TARGET="$2"; shift 2;;
        --cli-only) CLI_ONLY=true; shift;;
        --gui-only) GUI_ONLY=true; shift;;
        --release)  RELEASE=true; shift;;
        --help|-h)
            echo "Usage: ./build.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --target TARGET   Build target (macos-arm64, macos-amd64, linux-amd64, linux-arm64, windows-amd64, all)"
            echo "  --cli-only        Build only the CLI binary"
            echo "  --gui-only        Build only the GUI binary"
            echo "  --release         Build with optimizations"
            echo "  --help            Show this help"
            echo ""
            echo "Targets:"
            echo "  macos-arm64       macOS Apple Silicon (default on M-series Macs)"
            echo "  macos-amd64       macOS Intel"
            echo "  linux-amd64       Linux x86_64"
            echo "  linux-arm64       Linux ARM64 (Raspberry Pi, etc.)"
            echo "  windows-amd64     Windows x86_64"
            echo "  all               Build all targets"
            exit 0;;
        *) fail "Unknown option: $1";;
    esac
done

# Detect current platform if no target specified
if [[ -z "$TARGET" ]]; then
    case "$(uname -s)-$(uname -m)" in
        Darwin-arm64)  TARGET="macos-arm64";;
        Darwin-x86_64) TARGET="macos-amd64";;
        Linux-x86_64)  TARGET="linux-amd64";;
        Linux-aarch64) TARGET="linux-arm64";;
        *)             fail "Unsupported platform: $(uname -s)-$(uname -m)";;
    esac
    info "Auto-detected target: $TARGET"
fi

# Build flags
LDFLAGS="-s -w -X main.appVersion=$VERSION"
if $RELEASE; then
    GCFLAGS=""
else
    GCFLAGS=""
    LDFLAGS="-X main.appVersion=$VERSION"
fi

# ============================================================
# Build Zig quantum vault static library for a given target
# ============================================================
build_zig_lib() {
    local zig_target="$1"
    local lib_dir="$2"

    if [[ -f "$lib_dir/libquantum_vault.a" ]]; then
        ok "Zig library already exists: $lib_dir"
        return 0
    fi

    info "Building Zig quantum vault for $zig_target..."
    if [[ ! -d "$ZIG_CRYPTO" ]]; then
        fail "Zig crypto source not found at $ZIG_CRYPTO"
    fi

    mkdir -p "$lib_dir"

    local zig_args="-Doptimize=ReleaseFast"
    if [[ -n "$zig_target" ]]; then
        zig_args="$zig_args -Dtarget=$zig_target"
    fi

    (cd "$ZIG_CRYPTO" && $ZIG build $zig_args) || fail "Zig build failed for $zig_target"
    cp "$ZIG_CRYPTO/zig-out/lib/libquantum_vault.a" "$lib_dir/" || fail "Failed to copy Zig library"
    ok "Built Zig library → $lib_dir/libquantum_vault.a"
}

# ============================================================
# Build Go binary for a given target
# ============================================================
build_go() {
    local goos="$1"
    local goarch="$2"
    local suffix="$3"
    local build_what="$4"  # cli, gui, or both

    local out_dir="$OUTPUT_DIR/${goos}-${goarch}"
    mkdir -p "$out_dir"

    export CGO_ENABLED=1
    export GOOS="$goos"
    export GOARCH="$goarch"

    # Set cross-compiler for CGo if needed
    local current_os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local current_arch=$(uname -m)
    [[ "$current_arch" == "aarch64" ]] && current_arch="arm64"
    [[ "$current_arch" == "x86_64" ]] && current_arch="amd64"

    if [[ "$goos" != "$current_os" ]] || [[ "$goarch" != "$current_arch" ]]; then
        # Cross-compilation — need a C cross-compiler
        case "${goos}-${goarch}" in
            linux-amd64)
                if command -v x86_64-linux-gnu-gcc &>/dev/null; then
                    export CC=x86_64-linux-gnu-gcc
                elif command -v zig &>/dev/null; then
                    export CC="zig cc -target x86_64-linux-gnu"
                else
                    echo "  ⚠ No cross-compiler for linux-amd64 — skipping (install x86_64-linux-gnu-gcc or use zig)"
                    return 0
                fi
                ;;
            linux-arm64)
                if command -v aarch64-linux-gnu-gcc &>/dev/null; then
                    export CC=aarch64-linux-gnu-gcc
                else
                    echo "  ⚠ No cross-compiler for linux-arm64 — skipping"
                    return 0
                fi
                ;;
            darwin-*)
                if [[ "$current_os" != "darwin" ]]; then
                    echo "  ⚠ Cross-compiling to macOS requires macOS host — skipping"
                    return 0
                fi
                ;;
            windows-amd64)
                if command -v x86_64-w64-mingw32-gcc &>/dev/null; then
                    export CC=x86_64-w64-mingw32-gcc
                else
                    echo "  ⚠ No cross-compiler for windows — skipping (install mingw-w64)"
                    return 0
                fi
                ;;
        esac
    fi

    local ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"

    # Build CLI
    if [[ "$build_what" != "gui" ]] && ! $GUI_ONLY; then
        info "Building CLI → ${goos}/${goarch}"
        go build $GCFLAGS -ldflags "$LDFLAGS" \
            -o "$out_dir/ztransfer${ext}" \
            ./cmd/ztransfer/ 2>&1 || fail "CLI build failed for ${goos}/${goarch}"
        ok "CLI: $out_dir/ztransfer${ext}"
    fi

    # Build GUI
    if [[ "$build_what" != "cli" ]] && ! $CLI_ONLY; then
        info "Building GUI → ${goos}/${goarch}"
        go build $GCFLAGS -ldflags "$LDFLAGS" \
            -o "$out_dir/ztransfer-gui${ext}" \
            ./cmd/ztransfer-gui/ 2>&1 || fail "GUI build failed for ${goos}/${goarch}"
        ok "GUI: $out_dir/ztransfer-gui${ext}"

        # macOS: create .app bundle
        if [[ "$goos" == "darwin" ]]; then
            create_macos_app "$out_dir"
        fi
    fi

    unset CC GOOS GOARCH CGO_ENABLED
}

# ============================================================
# Create macOS .app bundle
# ============================================================
create_macos_app() {
    local out_dir="$1"
    local app_dir="$out_dir/ztransfer.app/Contents"

    mkdir -p "$app_dir/MacOS"
    mkdir -p "$app_dir/Resources"
    cp "$out_dir/ztransfer-gui" "$app_dir/MacOS/ztransfer"

    cat > "$app_dir/Info.plist" << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>ztransfer</string>
    <key>CFBundleIdentifier</key>
    <string>com.quantumencoding.ztransfer</string>
    <key>CFBundleName</key>
    <string>ztransfer</string>
    <key>CFBundleDisplayName</key>
    <string>ztransfer</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>CFBundleShortVersionString</key>
    <string>0.1.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.utilities</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

    ok "macOS app bundle: $out_dir/ztransfer.app"
}

# ============================================================
# Build for a specific target
# ============================================================
build_target() {
    local target="$1"

    case "$target" in
        macos-arm64)
            build_zig_lib "" "$PROJECT_DIR/libs/lib/darwin-arm64"
            build_go "darwin" "arm64" "" "both"
            ;;
        macos-amd64)
            build_zig_lib "x86_64-macos" "$PROJECT_DIR/libs/lib/darwin-amd64"
            build_go "darwin" "amd64" "" "both"
            ;;
        linux-amd64)
            build_zig_lib "x86_64-linux-gnu" "$PROJECT_DIR/libs/lib/linux-amd64"
            build_go "linux" "amd64" "" "both"
            ;;
        linux-arm64)
            build_zig_lib "aarch64-linux-gnu" "$PROJECT_DIR/libs/lib/linux-arm64"
            build_go "linux" "arm64" "" "both"
            ;;
        windows-amd64)
            build_zig_lib "x86_64-windows-gnu" "$PROJECT_DIR/libs/lib/windows-amd64"
            build_go "windows" "amd64" "" "both"
            ;;
        all)
            build_target "macos-arm64"
            build_target "macos-amd64"
            build_target "linux-amd64"
            build_target "linux-arm64"
            build_target "windows-amd64"
            ;;
        *)
            fail "Unknown target: $target (use --help for list)"
            ;;
    esac
}

# ============================================================
# Main
# ============================================================
echo ""
info "ztransfer build v${VERSION}"
echo ""

cd "$PROJECT_DIR"
build_target "$TARGET"

echo ""
info "Build complete!"
ls -lh "$OUTPUT_DIR"/*/ 2>/dev/null | grep -E '(ztransfer|\.app)' || true
echo ""
