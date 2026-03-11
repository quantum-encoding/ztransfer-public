ZIG_CRYPTO := ../quantum-zig-forge/programs/zig-quantum-encryption
ZIG := /usr/local/zig/zig

.PHONY: build build-cli build-gui build-zig clean test

# Build both CLI and GUI for current platform
build: build-cli build-gui

# Build CLI binary
build-cli: libs/lib/darwin-arm64/libquantum_vault.a
	CGO_ENABLED=1 go build -o dist/ztransfer ./cmd/ztransfer/

# Build GUI binary
build-gui: libs/lib/darwin-arm64/libquantum_vault.a
	CGO_ENABLED=1 go build -o dist/ztransfer-gui ./cmd/ztransfer-gui/

# Build Zig quantum vault for macOS (native)
build-zig:
	cd $(ZIG_CRYPTO) && $(ZIG) build -Doptimize=ReleaseFast
	mkdir -p libs/lib/darwin-arm64
	cp $(ZIG_CRYPTO)/zig-out/lib/libquantum_vault.a libs/lib/darwin-arm64/

# Ensure lib exists
libs/lib/darwin-arm64/libquantum_vault.a:
	$(MAKE) build-zig

# Cross-platform builds via build.sh
macos:   ; ./build.sh --target macos-arm64
linux:   ; ./build.sh --target linux-amd64
windows: ; ./build.sh --target windows-amd64
all:     ; ./build.sh --target all

test:
	CGO_ENABLED=1 go test ./...

clean:
	rm -rf dist/
	rm -f ztransfer ztransfer-gui

version:
	@dist/ztransfer version 2>/dev/null || go run ./cmd/ztransfer/ version
