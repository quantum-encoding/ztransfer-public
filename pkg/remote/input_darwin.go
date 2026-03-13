package remote

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// executeInput performs a mouse or keyboard action on macOS.
// Uses osascript (AppleScript) and cliclick when available, with
// CGEvent-based AppleScript as fallback.
func executeInput(action ComputerAction) error {
	switch action.Type {
	case ActionScreenshot:
		return nil // Handled separately via captureScreen

	case ActionClick:
		return mouseClick(action.X, action.Y, "left", false)

	case ActionDoubleClick:
		return mouseClick(action.X, action.Y, "left", true)

	case ActionRightClick:
		return mouseClick(action.X, action.Y, "right", false)

	case ActionMiddleClick:
		return mouseClick(action.X, action.Y, "middle", false)

	case ActionMove:
		return mouseMove(action.X, action.Y)

	case ActionDrag:
		return mouseDrag(action.StartX, action.StartY, action.EndX, action.EndY)

	case ActionKeyPress:
		return keyPress(action.Key)

	case ActionType_:
		return typeText(action.Text)

	case ActionScroll:
		return scroll(action.ScrollX, action.ScrollY, action.Direction, action.ScrollAmount)

	case ActionWait:
		time.Sleep(time.Second)
		return nil

	default:
		return fmt.Errorf("unsupported action type: %s", action.Type)
	}
}

func mouseClick(x, y int, button string, double bool) error {
	// Try cliclick first (faster, more reliable)
	if path, err := exec.LookPath("cliclick"); err == nil {
		var action string
		switch button {
		case "right":
			action = "rc"
		case "middle":
			action = "tc" // three-finger click
		default:
			if double {
				action = "dc"
			} else {
				action = "c"
			}
		}
		cmd := exec.Command(path, fmt.Sprintf("%s:%d,%d", action, x, y))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick %s: %w: %s", action, err, out)
		}
		return nil
	}

	// Fallback: AppleScript with System Events
	script := fmt.Sprintf(`
tell application "System Events"
	set mousePos to {%d, %d}
`, x, y)

	switch button {
	case "right":
		script += `	click at mousePos using {control down}
`
	default:
		if double {
			script += `	click at mousePos
	delay 0.05
	click at mousePos
`
		} else {
			script += `	click at mousePos
`
		}
	}
	script += `end tell`

	return runAppleScript(script)
}

func mouseMove(x, y int) error {
	if path, err := exec.LookPath("cliclick"); err == nil {
		cmd := exec.Command(path, fmt.Sprintf("m:%d,%d", x, y))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick move: %w: %s", err, out)
		}
		return nil
	}

	// AppleScript fallback — move mouse using Python bridge
	return runPythonInput(fmt.Sprintf(`
import Quartz
Quartz.CGEventPost(Quartz.kCGHIDEventTap,
    Quartz.CGEventCreateMouseEvent(None, Quartz.kCGEventMouseMoved, (%d, %d), 0))
`, x, y))
}

func mouseDrag(startX, startY, endX, endY int) error {
	if path, err := exec.LookPath("cliclick"); err == nil {
		cmd := exec.Command(path,
			fmt.Sprintf("dd:%d,%d", startX, startY),
			fmt.Sprintf("du:%d,%d", endX, endY),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick drag: %w: %s", err, out)
		}
		return nil
	}

	return runPythonInput(fmt.Sprintf(`
import Quartz, time
Quartz.CGEventPost(Quartz.kCGHIDEventTap,
    Quartz.CGEventCreateMouseEvent(None, Quartz.kCGEventLeftMouseDown, (%d, %d), 0))
time.sleep(0.05)
Quartz.CGEventPost(Quartz.kCGHIDEventTap,
    Quartz.CGEventCreateMouseEvent(None, Quartz.kCGEventLeftMouseDragged, (%d, %d), 0))
time.sleep(0.05)
Quartz.CGEventPost(Quartz.kCGHIDEventTap,
    Quartz.CGEventCreateMouseEvent(None, Quartz.kCGEventLeftMouseUp, (%d, %d), 0))
`, startX, startY, endX, endY, endX, endY))
}

func keyPress(key string) error {
	if path, err := exec.LookPath("cliclick"); err == nil {
		// cliclick uses kp: for key presses
		cliKey := mapKeyToCliclick(key)
		cmd := exec.Command(path, "kp:"+cliKey)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick keypress: %w: %s", err, out)
		}
		return nil
	}

	// AppleScript fallback
	appleKey := mapKeyToAppleScript(key)
	if strings.HasPrefix(appleKey, "keycode:") {
		code := strings.TrimPrefix(appleKey, "keycode:")
		return runAppleScript(fmt.Sprintf(`
tell application "System Events"
	key code %s
end tell`, code))
	}

	return runAppleScript(fmt.Sprintf(`
tell application "System Events"
	keystroke "%s"
end tell`, appleKey))
}

func typeText(text string) error {
	if path, err := exec.LookPath("cliclick"); err == nil {
		cmd := exec.Command(path, "t:"+text)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick type: %w: %s", err, out)
		}
		return nil
	}

	// AppleScript — escape for special characters
	escaped := strings.ReplaceAll(text, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return runAppleScript(fmt.Sprintf(`
tell application "System Events"
	keystroke "%s"
end tell`, escaped))
}

func scroll(x, y int, direction string, amount int) error {
	if amount == 0 {
		amount = 3
	}

	// First move to position if specified
	if x > 0 || y > 0 {
		mouseMove(x, y)
	}

	var scrollDir int
	switch direction {
	case "up":
		scrollDir = amount
	case "down":
		scrollDir = -amount
	case "left":
		scrollDir = amount // horizontal handled separately
	case "right":
		scrollDir = -amount
	default:
		scrollDir = -amount // default down
	}

	if direction == "left" || direction == "right" {
		return runPythonInput(fmt.Sprintf(`
import Quartz
event = Quartz.CGEventCreateScrollWheelEvent(None, Quartz.kCGScrollEventUnitLine, 2, 0, %d)
Quartz.CGEventPost(Quartz.kCGHIDEventTap, event)
`, scrollDir))
	}

	return runPythonInput(fmt.Sprintf(`
import Quartz
event = Quartz.CGEventCreateScrollWheelEvent(None, Quartz.kCGScrollEventUnitLine, 1, %d)
Quartz.CGEventPost(Quartz.kCGHIDEventTap, event)
`, scrollDir))
}

func runAppleScript(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w: %s", err, out)
	}
	return nil
}

func runPythonInput(script string) error {
	// Try python3 with PyObjC (ships with macOS)
	cmd := exec.Command("python3", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("python3 input: %w: %s", err, out)
	}
	return nil
}

// mapKeyToCliclick maps standard key names to cliclick format.
func mapKeyToCliclick(key string) string {
	keyMap := map[string]string{
		"Return": "return", "Enter": "return",
		"Tab": "tab", "Escape": "escape", "Space": "space",
		"Backspace": "delete", "Delete": "fwd-delete",
		"Up": "arrow-up", "Down": "arrow-down",
		"Left": "arrow-left", "Right": "arrow-right",
		"Home": "home", "End": "end",
		"PageUp": "page-up", "PageDown": "page-down",
		"F1": "f1", "F2": "f2", "F3": "f3", "F4": "f4",
		"F5": "f5", "F6": "f6", "F7": "f7", "F8": "f8",
		"F9": "f9", "F10": "f10", "F11": "f11", "F12": "f12",
	}
	if mapped, ok := keyMap[key]; ok {
		return mapped
	}
	return key
}

// mapKeyToAppleScript maps key names to AppleScript keystroke values.
func mapKeyToAppleScript(key string) string {
	keyMap := map[string]string{
		"Return": "keycode:36", "Enter": "keycode:36",
		"Tab": "keycode:48", "Escape": "keycode:53",
		"Space": " ",
		"Backspace": "keycode:51", "Delete": "keycode:117",
		"Up": "keycode:126", "Down": "keycode:125",
		"Left": "keycode:123", "Right": "keycode:124",
		"Home": "keycode:115", "End": "keycode:119",
		"PageUp": "keycode:116", "PageDown": "keycode:121",
	}
	if mapped, ok := keyMap[key]; ok {
		return mapped
	}
	// For F-keys
	fKeyMap := map[string]string{
		"F1": "keycode:122", "F2": "keycode:120", "F3": "keycode:99",
		"F4": "keycode:118", "F5": "keycode:96", "F6": "keycode:97",
		"F7": "keycode:98", "F8": "keycode:100", "F9": "keycode:101",
		"F10": "keycode:109", "F11": "keycode:103", "F12": "keycode:111",
	}
	if mapped, ok := fKeyMap[key]; ok {
		return mapped
	}
	return key
}

// hasAccessibilityPermission checks if the process has accessibility access.
// On macOS, screen control requires the app to be approved in
// System Preferences > Security & Privacy > Privacy > Accessibility.
func hasAccessibilityPermission() bool {
	// tccutil doesn't have a query mode, but we can try a trivial action
	cmd := exec.Command("osascript", "-e", `tell application "System Events" to key code 0`)
	err := cmd.Run()
	return err == nil
}

