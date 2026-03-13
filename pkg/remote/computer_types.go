package remote

// ComputerAction represents a mouse, keyboard, or screen action to execute
// on a remote machine. This is the normalized format used over the wire —
// AI providers (Anthropic, etc.) map their native action formats to this.
type ComputerAction struct {
	Type ActionType `json:"type"`

	// Mouse fields
	X      int `json:"x,omitempty"`
	Y      int `json:"y,omitempty"`
	Button string `json:"button,omitempty"` // "left", "right", "middle"

	// Drag fields
	StartX int `json:"start_x,omitempty"`
	StartY int `json:"start_y,omitempty"`
	EndX   int `json:"end_x,omitempty"`
	EndY   int `json:"end_y,omitempty"`

	// Keyboard fields
	Key  string `json:"key,omitempty"`  // Single key name ("Return", "Tab", "a", etc.)
	Text string `json:"text,omitempty"` // Text to type

	// Scroll fields
	ScrollX     int    `json:"scroll_x,omitempty"`     // Scroll position X
	ScrollY     int    `json:"scroll_y,omitempty"`      // Scroll position Y
	Direction   string `json:"direction,omitempty"`     // "up", "down", "left", "right"
	ScrollAmount int   `json:"scroll_amount,omitempty"` // Number of scroll units

	// Screenshot fields
	Format string `json:"format,omitempty"` // "png" (default), "jpeg"
}

// ActionType classifies the kind of computer action.
type ActionType string

const (
	ActionScreenshot  ActionType = "screenshot"
	ActionClick       ActionType = "click"
	ActionDoubleClick ActionType = "double_click"
	ActionRightClick  ActionType = "right_click"
	ActionMiddleClick ActionType = "middle_click"
	ActionMove        ActionType = "move"
	ActionDrag        ActionType = "drag"
	ActionKeyPress    ActionType = "key"
	ActionType_       ActionType = "type" // Type text
	ActionScroll      ActionType = "scroll"
	ActionWait        ActionType = "wait"
)

// ActionResult is the response after executing a ComputerAction.
type ActionResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ScreenInfo describes the remote display.
type ScreenInfo struct {
	Width  int     `json:"width"`       // Logical width in pixels
	Height int     `json:"height"`      // Logical height in pixels
	Scale  float64 `json:"scale"`       // Display scale factor (2.0 for Retina)
	OS     string  `json:"os"`          // "darwin", "linux", "windows"
}

// ScreenRequest configures what kind of screenshot to capture.
type ScreenRequest struct {
	Format  string `json:"format,omitempty"`  // "png" (default), "jpeg"
	Quality int    `json:"quality,omitempty"` // JPEG quality 1-100 (default 80)
	Scale   int    `json:"scale,omitempty"`   // Downscale factor (default 1)
}
