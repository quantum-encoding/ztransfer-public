package remote

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// captureScreen takes a screenshot on Linux.
// Tries in order: grim (Wayland), import (X11/ImageMagick), scrot, gnome-screenshot.
func captureScreen(req ScreenRequest) ([]byte, error) {
	ext := "png"
	if req.Format == "jpeg" {
		ext = "jpg"
	}

	tmpFile, err := os.CreateTemp("", "ztransfer-screen-*."+ext)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Detect display server
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	xDisplay := os.Getenv("DISPLAY")

	var captureErr error

	if waylandDisplay != "" {
		// Wayland: try grim
		cmd := exec.Command("grim", tmpPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			return readAndReturn(tmpPath)
		} else {
			captureErr = fmt.Errorf("grim: %w: %s", err, out)
		}
	}

	if xDisplay != "" {
		// X11: try import (ImageMagick)
		cmd := exec.Command("import", "-window", "root", tmpPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			return readAndReturn(tmpPath)
		} else {
			captureErr = fmt.Errorf("import: %w: %s", err, out)
		}

		// X11: try scrot
		cmd = exec.Command("scrot", tmpPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			return readAndReturn(tmpPath)
		} else {
			captureErr = fmt.Errorf("scrot: %w: %s", err, out)
		}
	}

	// Last resort: gnome-screenshot
	cmd := exec.Command("gnome-screenshot", "-f", tmpPath)
	if out, err := cmd.CombinedOutput(); err == nil {
		return readAndReturn(tmpPath)
	} else {
		captureErr = fmt.Errorf("gnome-screenshot: %w: %s", err, out)
	}

	return nil, fmt.Errorf("no screenshot tool available (tried grim, import, scrot, gnome-screenshot): last error: %v", captureErr)
}

func readAndReturn(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}
	return data, nil
}

// getScreenInfo returns display information on Linux.
func getScreenInfo() (ScreenInfo, error) {
	info := ScreenInfo{
		OS:    runtime.GOOS,
		Scale: 1.0,
	}

	// Try xrandr for X11
	cmd := exec.Command("xrandr", "--current")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			// Look for connected primary display with resolution
			if strings.Contains(line, " connected") {
				// Format: "DP-1 connected primary 2560x1440+0+0 ..."
				fields := strings.Fields(line)
				for _, f := range fields {
					if strings.Contains(f, "x") && strings.Contains(f, "+") {
						// "2560x1440+0+0"
						res := strings.Split(f, "+")[0]
						parts := strings.Split(res, "x")
						if len(parts) == 2 {
							w, _ := strconv.Atoi(parts[0])
							h, _ := strconv.Atoi(parts[1])
							if w > 0 && h > 0 {
								info.Width = w
								info.Height = h
								return info, nil
							}
						}
					}
				}
			}
		}
	}

	// Try wlr-randr for Wayland
	cmd = exec.Command("wlr-randr")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// "2560x1440 px, 59.951000 Hz (preferred, current)"
			if strings.Contains(line, "px") && strings.Contains(line, "current") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					res := strings.Split(parts[0], "x")
					if len(res) == 2 {
						w, _ := strconv.Atoi(res[0])
						h, _ := strconv.Atoi(res[1])
						if w > 0 && h > 0 {
							info.Width = w
							info.Height = h
							return info, nil
						}
					}
				}
			}
		}
	}

	// Fallback
	info.Width = 1920
	info.Height = 1080
	return info, nil
}
