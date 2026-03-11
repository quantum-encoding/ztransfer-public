package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/quantum-encoding/ztransfer-public/pkg/nat"
)

// ExecRequest represents a command to execute remotely.
type ExecRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// ExecResponse is the result of remote command execution.
type ExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ExecServer handles an exec request on the remote side.
// It runs the requested command and returns the collected output.
func ExecServer(tunnel *nat.Tunnel, req ExecRequest) ExecResponse {
	if req.Command == "" {
		return ExecResponse{
			Stderr:   "empty command",
			ExitCode: 1,
		}
	}

	cmd := exec.Command(req.Command, req.Args...)

	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	if len(req.Env) > 0 {
		cmd.Env = req.Env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecResponse{
				Stderr:   fmt.Sprintf("exec error: %v", err),
				ExitCode: 1,
			}
		}
	}

	return ExecResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// ExecClient sends an exec request through the tunnel and waits for the
// response. The request is serialized as JSON in a MsgExecReq message and
// the response is read from a MsgExecResp message.
func ExecClient(tunnel *nat.Tunnel, req ExecRequest) (*ExecResponse, error) {
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal exec request: %w", err)
	}

	router := NewRouter(tunnel)
	respCh := make(chan *ExecResponse, 1)
	errCh := make(chan error, 1)

	router.Handle(MsgExecResp, func(payload []byte) {
		var resp ExecResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			errCh <- fmt.Errorf("unmarshal exec response: %w", err)
			return
		}
		respCh <- &resp
	})

	router.Handle(MsgError, func(payload []byte) {
		errCh <- fmt.Errorf("remote error: %s", string(payload))
	})

	// Start router in background.
	go router.Start()
	defer router.Stop()

	// Send the exec request.
	if err := router.Send(MsgExecReq, reqData); err != nil {
		return nil, fmt.Errorf("send exec request: %w", err)
	}

	// Wait for response.
	select {
	case resp := <-respCh:
		return resp, nil
	case err := <-errCh:
		return nil, err
	case <-tunnel.Done():
		return nil, fmt.Errorf("tunnel closed while waiting for exec response")
	}
}

// HandleExecRequests registers an exec request handler on the router.
// When a MsgExecReq is received, the command is executed and the result
// sent back as a MsgExecResp. This is used by the server side.
func HandleExecRequests(router *MessageRouter, tunnel *nat.Tunnel) {
	router.Handle(MsgExecReq, func(payload []byte) {
		var req ExecRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			errMsg := fmt.Sprintf("invalid exec request: %v", err)
			router.Send(MsgError, []byte(errMsg))
			return
		}

		resp := ExecServer(tunnel, req)
		respData, err := json.Marshal(resp)
		if err != nil {
			errMsg := fmt.Sprintf("marshal exec response: %v", err)
			router.Send(MsgError, []byte(errMsg))
			return
		}
		router.Send(MsgExecResp, respData)
	})
}
