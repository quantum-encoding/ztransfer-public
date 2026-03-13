//go:build darwin || linux

package remote

// Public wrappers for screen capture and input injection.
// Used by the local test viewer and any code that needs direct
// access without going through a tunnel.

// CaptureScreenPublic captures a screenshot of the local machine.
func CaptureScreenPublic(req ScreenRequest) ([]byte, error) {
	return captureScreen(req)
}

// CaptureScreenJPEGPublic captures and compresses a screenshot.
func CaptureScreenJPEGPublic(quality int, downscale int) ([]byte, error) {
	return CaptureScreenJPEG(quality, downscale)
}

// ExecuteInputPublic executes an input action on the local machine.
func ExecuteInputPublic(action ComputerAction) error {
	return executeInput(action)
}

// GetScreenInfoPublic returns display info for the local machine.
func GetScreenInfoPublic() (ScreenInfo, error) {
	return getScreenInfo()
}
