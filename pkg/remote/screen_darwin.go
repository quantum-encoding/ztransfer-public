package remote

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// captureScreen takes a screenshot on macOS using the screencapture command.
// Returns PNG bytes.
func captureScreen(req ScreenRequest) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "ztransfer-screen-*.png")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// screencapture: -x = no sound, -C = capture cursor, -t = format
	args := []string{"-x", "-C"}

	format := req.Format
	if format == "" || format == "png" {
		args = append(args, "-t", "png")
	} else if format == "jpeg" {
		args = append(args, "-t", "jpg")
	}

	args = append(args, tmpPath)

	cmd := exec.Command("screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screencapture failed: %w: %s", err, out)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	return data, nil
}

// getScreenInfo returns display information on macOS.
func getScreenInfo() (ScreenInfo, error) {
	return getScreenInfoFallback()
}

func getScreenInfoFallback() (ScreenInfo, error) {
	info := ScreenInfo{
		OS:    runtime.GOOS,
		Scale: 1.0,
	}

	// Try using osascript to get screen bounds
	cmd := exec.Command("osascript", "-e",
		`tell application "Finder" to get bounds of window of desktop`)
	out, err := cmd.Output()
	if err == nil {
		// Output format: "0, 0, 1920, 1080"
		parts := strings.Split(strings.TrimSpace(string(out)), ", ")
		if len(parts) == 4 {
			w, _ := strconv.Atoi(parts[2])
			h, _ := strconv.Atoi(parts[3])
			if w > 0 && h > 0 {
				info.Width = w
				info.Height = h
			}
		}
	}

	// If that failed, try screenresolution
	if info.Width == 0 {
		cmd = exec.Command("system_profiler", "SPDisplaysDataType")
		out, err = cmd.Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Resolution:") {
					// "Resolution: 2560 x 1440 Retina"
					line = strings.TrimPrefix(line, "Resolution:")
					line = strings.TrimSpace(line)
					parts := strings.Fields(line)
					if len(parts) >= 3 {
						w, _ := strconv.Atoi(parts[0])
						h, _ := strconv.Atoi(parts[2])
						if w > 0 && h > 0 {
							info.Width = w
							info.Height = h
							if strings.Contains(line, "Retina") {
								info.Scale = 2.0
							}
						}
					}
					break
				}
			}
		}
	}

	// Ultimate fallback
	if info.Width == 0 {
		info.Width = 1920
		info.Height = 1080
	}

	return info, nil
}
