// Package remote provides interactive shell, command execution, and session
// management over encrypted ztransfer tunnels.
package remote

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/quantum-encoding/ztransfer-public/pkg/nat"
)

// Message types for the shell protocol.
const (
	MsgStdout   byte = 0x01
	MsgStdin    byte = 0x02
	MsgResize   byte = 0x03
	MsgExit     byte = 0x04
	MsgError    byte = 0x05
	MsgExecReq  byte = 0x10
	MsgExecResp byte = 0x11
	MsgPing     byte = 0x20
	MsgPong     byte = 0x21
)

// ShellMessage is the wire format for shell data.
type ShellMessage struct {
	Type    byte
	Payload []byte
}

// EncodeMessage encodes a shell message to bytes: [type][len_u32_be][payload].
func EncodeMessage(msg ShellMessage) []byte {
	length := len(msg.Payload)
	buf := make([]byte, 1+4+length)
	buf[0] = msg.Type
	binary.BigEndian.PutUint32(buf[1:5], uint32(length))
	copy(buf[5:], msg.Payload)
	return buf
}

// DecodeMessage decodes a shell message from bytes.
func DecodeMessage(data []byte) (ShellMessage, error) {
	if len(data) < 5 {
		return ShellMessage{}, fmt.Errorf("decode message: data too short (%d bytes)", len(data))
	}
	msgType := data[0]
	length := binary.BigEndian.Uint32(data[1:5])
	if uint32(len(data)-5) < length {
		return ShellMessage{}, fmt.Errorf("decode message: payload truncated (have %d, want %d)", len(data)-5, length)
	}
	return ShellMessage{
		Type:    msgType,
		Payload: data[5 : 5+length],
	}, nil
}

// MessageRouter multiplexes different message types over a single tunnel.
// It reads framed messages from the tunnel, examines the type byte, and
// dispatches the payload to the registered handler.
type MessageRouter struct {
	tunnel   *nat.Tunnel
	handlers map[byte]func([]byte)
	mu       sync.RWMutex
	done     chan struct{}
	stopOnce sync.Once
}

// NewRouter creates a message router for a tunnel.
func NewRouter(tunnel *nat.Tunnel) *MessageRouter {
	return &MessageRouter{
		tunnel:   tunnel,
		handlers: make(map[byte]func([]byte)),
		done:     make(chan struct{}),
	}
}

// Handle registers a handler for a message type.
// Must be called before Start.
func (r *MessageRouter) Handle(msgType byte, handler func([]byte)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[msgType] = handler
}

// Send sends a typed message through the tunnel.
// The wire format is [type_byte][payload], wrapped in the tunnel's
// length-prefixed framing.
func (r *MessageRouter) Send(msgType byte, payload []byte) error {
	msg := EncodeMessage(ShellMessage{Type: msgType, Payload: payload})
	return r.tunnel.Send(msg)
}

// Start begins routing incoming messages to registered handlers.
// It blocks until the tunnel is closed or Stop is called.
func (r *MessageRouter) Start() error {
	for {
		select {
		case <-r.done:
			return nil
		default:
		}

		data, err := r.tunnel.Recv()
		if err != nil {
			select {
			case <-r.done:
				return nil
			default:
				return fmt.Errorf("router recv: %w", err)
			}
		}

		msg, err := DecodeMessage(data)
		if err != nil {
			continue // skip malformed messages
		}

		r.mu.RLock()
		handler, ok := r.handlers[msg.Type]
		r.mu.RUnlock()

		if ok {
			handler(msg.Payload)
		}
	}
}

// Stop stops the router and closes the underlying tunnel.
func (r *MessageRouter) Stop() {
	r.stopOnce.Do(func() {
		close(r.done)
		r.tunnel.Close()
	})
}
