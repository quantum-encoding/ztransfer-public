//go:build darwin || linux

package remote

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/quantum-encoding/ztransfer/pkg/audit"
	"github.com/quantum-encoding/ztransfer/pkg/nat"
)

// ComputerSession manages a computer use session over an encrypted tunnel.
// The server side captures screens and executes input actions.
// The client side sends requests and receives results via the tunnel.
type ComputerSession struct {
	tunnel *nat.Tunnel
	router *MessageRouter
	audit  *audit.Chain
	info   ScreenInfo

	mu     sync.Mutex
	closed bool
}

// ServeComputer starts the server side of a computer use session.
// It registers handlers for screen capture and input injection on the
// remote machine, then blocks until the session ends.
func ServeComputer(tunnel *nat.Tunnel, auditChain *audit.Chain) error {
	info, err := getScreenInfo()
	if err != nil {
		return fmt.Errorf("get screen info: %w", err)
	}

	cs := &ComputerSession{
		tunnel: tunnel,
		router: NewRouter(tunnel),
		audit:  auditChain,
		info:   info,
	}

	// Register handlers
	cs.router.Handle(MsgScreenReq, cs.handleScreenReq)
	cs.router.Handle(MsgInputReq, cs.handleInputReq)
	cs.router.Handle(MsgScreenInfo, cs.handleScreenInfoReq)

	// Send initial screen info to client
	infoData, _ := json.Marshal(info)
	cs.router.Send(MsgScreenInfo, infoData)

	if auditChain != nil {
		auditChain.Append(&audit.Event{
			EventType:   audit.EventSessionStart,
			Description: "Computer use session started",
			Metadata: map[string]string{
				"screen_width":  fmt.Sprintf("%d", info.Width),
				"screen_height": fmt.Sprintf("%d", info.Height),
				"screen_scale":  fmt.Sprintf("%.1f", info.Scale),
				"os":            info.OS,
			},
		})
	}

	// Block until tunnel closes
	return cs.router.Start()
}

func (cs *ComputerSession) handleScreenReq(payload []byte) {
	var req ScreenRequest
	if len(payload) > 0 {
		json.Unmarshal(payload, &req)
	}

	var data []byte
	var err error

	// Use JPEG compression for viewer traffic (format=jpeg or quality set)
	if req.Format == "jpeg" || req.Quality > 0 {
		quality := req.Quality
		if quality <= 0 {
			quality = 65
		}
		scale := req.Scale
		if scale <= 0 {
			scale = 2 // Default: halve Retina resolution
		}
		data, err = CaptureScreenJPEG(quality, scale)
	} else {
		data, err = captureScreen(req)
	}

	if err != nil {
		errResp, _ := json.Marshal(ActionResult{
			Success: false,
			Error:   err.Error(),
		})
		cs.router.Send(MsgScreenResp, errResp)
		return
	}

	if cs.audit != nil {
		cs.audit.Append(&audit.Event{
			EventType:   audit.EventHeartbeat,
			Description: "Screenshot captured",
			ByteCount:   int64(len(data)),
		})
	}

	cs.router.Send(MsgScreenResp, data)
}

func (cs *ComputerSession) handleInputReq(payload []byte) {
	var action ComputerAction
	if err := json.Unmarshal(payload, &action); err != nil {
		resp, _ := json.Marshal(ActionResult{
			Success: false,
			Error:   "invalid action: " + err.Error(),
		})
		cs.router.Send(MsgInputResp, resp)
		return
	}

	if cs.audit != nil {
		meta := map[string]string{
			"action_type": string(action.Type),
		}
		if action.X > 0 || action.Y > 0 {
			meta["x"] = fmt.Sprintf("%d", action.X)
			meta["y"] = fmt.Sprintf("%d", action.Y)
		}
		if action.Text != "" {
			meta["text_length"] = fmt.Sprintf("%d", len(action.Text))
		}
		if action.Key != "" {
			meta["key"] = action.Key
		}
		if action.Direction != "" {
			meta["direction"] = action.Direction
		}

		// Log the command but redact typed text for privacy
		desc := fmt.Sprintf("Input: %s", action.Type)
		if action.Type == ActionClick || action.Type == ActionDoubleClick || action.Type == ActionRightClick {
			desc = fmt.Sprintf("Input: %s at (%d, %d)", action.Type, action.X, action.Y)
		} else if action.Type == ActionKeyPress {
			desc = fmt.Sprintf("Input: key %s", action.Key)
		} else if action.Type == ActionType_ {
			desc = fmt.Sprintf("Input: type %d chars", len(action.Text))
		}

		cs.audit.Append(&audit.Event{
			EventType:   audit.EventCommandExec,
			Description: desc,
			Command:     string(action.Type),
			Metadata:    meta,
		})
	}

	err := executeInput(action)

	resp := ActionResult{Success: err == nil}
	if err != nil {
		resp.Error = err.Error()
	}

	respData, _ := json.Marshal(resp)
	cs.router.Send(MsgInputResp, respData)
}

func (cs *ComputerSession) handleScreenInfoReq(payload []byte) {
	data, _ := json.Marshal(cs.info)
	cs.router.Send(MsgScreenInfo, data)
}

// --------------------------------------------------------------------------
// Client side — used by the API server and ComputerProvider
// --------------------------------------------------------------------------

// ComputerClient is the client side of a computer use session.
// It sends screen capture and input requests over the tunnel.
type ComputerClient struct {
	tunnel *nat.Tunnel
	router *MessageRouter
	Info   ScreenInfo

	screenCh chan []byte
	inputCh  chan ActionResult
	infoCh   chan ScreenInfo

	mu     sync.Mutex
	closed bool
}

// NewComputerClient creates a client for controlling a remote computer.
func NewComputerClient(tunnel *nat.Tunnel) *ComputerClient {
	cc := &ComputerClient{
		tunnel:   tunnel,
		router:   NewRouter(tunnel),
		screenCh: make(chan []byte, 1),
		inputCh:  make(chan ActionResult, 1),
		infoCh:   make(chan ScreenInfo, 1),
	}

	cc.router.Handle(MsgScreenResp, func(payload []byte) {
		// Check if it's an error response (JSON) or raw PNG data
		if len(payload) > 0 && payload[0] == '{' {
			var result ActionResult
			if json.Unmarshal(payload, &result) == nil && !result.Success {
				cc.screenCh <- nil
				return
			}
		}
		cc.screenCh <- payload
	})

	cc.router.Handle(MsgInputResp, func(payload []byte) {
		var result ActionResult
		if err := json.Unmarshal(payload, &result); err != nil {
			cc.inputCh <- ActionResult{Success: false, Error: err.Error()}
			return
		}
		cc.inputCh <- result
	})

	cc.router.Handle(MsgScreenInfo, func(payload []byte) {
		var info ScreenInfo
		if err := json.Unmarshal(payload, &info); err == nil {
			cc.Info = info
			select {
			case cc.infoCh <- info:
			default:
			}
		}
	})

	go cc.router.Start()

	return cc
}

// Screenshot requests a screenshot from the remote machine.
// Returns raw PNG bytes.
func (cc *ComputerClient) Screenshot(req ScreenRequest) ([]byte, error) {
	reqData, _ := json.Marshal(req)
	if err := cc.router.Send(MsgScreenReq, reqData); err != nil {
		return nil, fmt.Errorf("send screen request: %w", err)
	}

	select {
	case data := <-cc.screenCh:
		if data == nil {
			return nil, fmt.Errorf("screenshot failed on remote")
		}
		return data, nil
	case <-cc.tunnel.Done():
		return nil, fmt.Errorf("tunnel closed")
	}
}

// Execute sends an input action to the remote machine.
func (cc *ComputerClient) Execute(action ComputerAction) (ActionResult, error) {
	data, _ := json.Marshal(action)
	if err := cc.router.Send(MsgInputReq, data); err != nil {
		return ActionResult{}, fmt.Errorf("send input request: %w", err)
	}

	select {
	case result := <-cc.inputCh:
		return result, nil
	case <-cc.tunnel.Done():
		return ActionResult{}, fmt.Errorf("tunnel closed")
	}
}

// GetScreenInfo requests display info from the remote machine.
func (cc *ComputerClient) GetScreenInfo() (ScreenInfo, error) {
	if err := cc.router.Send(MsgScreenInfo, nil); err != nil {
		return ScreenInfo{}, fmt.Errorf("send screen info request: %w", err)
	}

	select {
	case info := <-cc.infoCh:
		return info, nil
	case <-cc.tunnel.Done():
		return ScreenInfo{}, fmt.Errorf("tunnel closed")
	}
}

// Close stops the client and underlying router.
func (cc *ComputerClient) Close() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if !cc.closed {
		cc.closed = true
		cc.router.Stop()
	}
}
