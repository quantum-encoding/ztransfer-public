package remote

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// executeInput performs a mouse or keyboard action on Linux.
// Uses xdotool (X11) or ydotool (Wayland), with wtype as fallback.
func executeInput(action ComputerAction) error {
	switch action.Type {
	case ActionScreenshot:
		return nil // Handled separately

	case ActionClick:
		return xMouseClick(action.X, action.Y, 1, false)

	case ActionDoubleClick:
		return xMouseClick(action.X, action.Y, 1, true)

	case ActionRightClick:
		return xMouseClick(action.X, action.Y, 3, false)

	case ActionMiddleClick:
		return xMouseClick(action.X, action.Y, 2, false)

	case ActionMove:
		return xMouseMove(action.X, action.Y)

	case ActionDrag:
		return xMouseDrag(action.StartX, action.StartY, action.EndX, action.EndY)

	case ActionKeyPress:
		return xKeyPress(action.Key)

	case ActionType_:
		return xTypeText(action.Text)

	case ActionScroll:
		return xScroll(action.ScrollX, action.ScrollY, action.Direction, action.ScrollAmount)

	case ActionWait:
		time.Sleep(time.Second)
		return nil

	default:
		return fmt.Errorf("unsupported action type: %s", action.Type)
	}
}

// isWayland returns true if running under a Wayland compositor.
func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func xMouseMove(x, y int) error {
	if isWayland() {
		return ydotool("mousemove", "--absolute", "-x", itoa(x), "-y", itoa(y))
	}
	return xdotool("mousemove", "--", itoa(x), itoa(y))
}

func xMouseClick(x, y int, button int, double bool) error {
	if isWayland() {
		if err := ydotool("mousemove", "--absolute", "-x", itoa(x), "-y", itoa(y)); err != nil {
			return err
		}
		args := []string{"click", itoa(buttonToYdotool(button))}
		if double {
			args = append(args, itoa(buttonToYdotool(button)))
		}
		return ydotool(args...)
	}

	args := []string{"mousemove", "--", itoa(x), itoa(y), "click", itoa(button)}
	if double {
		args = []string{"mousemove", "--", itoa(x), itoa(y), "click", "--repeat", "2", "--delay", "50", itoa(button)}
	}
	return xdotool(args...)
}

func xMouseDrag(startX, startY, endX, endY int) error {
	if isWayland() {
		// ydotool doesn't have native drag, simulate with events
		if err := ydotool("mousemove", "--absolute", "-x", itoa(startX), "-y", itoa(startY)); err != nil {
			return err
		}
		if err := ydotool("mousedown", "1"); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		if err := ydotool("mousemove", "--absolute", "-x", itoa(endX), "-y", itoa(endY)); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		return ydotool("mouseup", "1")
	}

	return xdotool(
		"mousemove", "--", itoa(startX), itoa(startY),
		"mousedown", "1",
		"mousemove", "--", itoa(endX), itoa(endY),
		"mouseup", "1",
	)
}

func xKeyPress(key string) error {
	xKey := mapKeyToX11(key)
	if isWayland() {
		return ydotool("key", xKey)
	}
	return xdotool("key", "--", xKey)
}

func xTypeText(text string) error {
	if isWayland() {
		// Try wtype first (better Wayland text input)
		if path, err := exec.LookPath("wtype"); err == nil {
			cmd := exec.Command(path, text)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("wtype: %w: %s", err, out)
			}
			return nil
		}
		return ydotool("type", "--", text)
	}
	return xdotool("type", "--clearmodifiers", "--delay", "12", "--", text)
}

func xScroll(x, y int, direction string, amount int) error {
	if amount == 0 {
		amount = 3
	}

	// Move to position first if specified
	if x > 0 || y > 0 {
		xMouseMove(x, y)
	}

	if isWayland() {
		// ydotool scroll: positive = down, negative = up
		var scrollVal int
		switch direction {
		case "up":
			scrollVal = -amount
		case "down":
			scrollVal = amount
		default:
			scrollVal = amount
		}
		return ydotool("mousemove", "--wheel", itoa(scrollVal))
	}

	// xdotool: button 4 = scroll up, button 5 = scroll down
	// button 6 = scroll left, button 7 = scroll right
	var button int
	switch direction {
	case "up":
		button = 4
	case "down":
		button = 5
	case "left":
		button = 6
	case "right":
		button = 7
	default:
		button = 5 // default down
	}

	return xdotool("click", "--repeat", itoa(amount), "--delay", "20", itoa(button))
}

func xdotool(args ...string) error {
	cmd := exec.Command("xdotool", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xdotool %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

func ydotool(args ...string) error {
	cmd := exec.Command("ydotool", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ydotool %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

// buttonToYdotool maps X11 button numbers to ydotool button codes.
// ydotool uses hex codes: 0x00 = left, 0x01 = right, 0x02 = middle
func buttonToYdotool(xButton int) int {
	switch xButton {
	case 1:
		return 0x00 // left
	case 2:
		return 0x02 // middle
	case 3:
		return 0x01 // right
	default:
		return 0x00
	}
}

// mapKeyToX11 maps standard key names to X11 keysym names.
func mapKeyToX11(key string) string {
	keyMap := map[string]string{
		"Return": "Return", "Enter": "Return",
		"Tab": "Tab", "Escape": "Escape", "Space": "space",
		"Backspace": "BackSpace", "Delete": "Delete",
		"Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right",
		"Home": "Home", "End": "End",
		"PageUp": "Prior", "PageDown": "Next",
		"F1": "F1", "F2": "F2", "F3": "F3", "F4": "F4",
		"F5": "F5", "F6": "F6", "F7": "F7", "F8": "F8",
		"F9": "F9", "F10": "F10", "F11": "F11", "F12": "F12",
		"Control_L": "Control_L", "Control_R": "Control_R",
		"Alt_L": "Alt_L", "Alt_R": "Alt_R",
		"Shift_L": "Shift_L", "Shift_R": "Shift_R",
		"Super_L": "Super_L", "Super_R": "Super_R",
	}
	if mapped, ok := keyMap[key]; ok {
		return mapped
	}
	return key
}
