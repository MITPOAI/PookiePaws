package gateway

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type websocketConn struct {
	conn net.Conn
	rw   *bufio.ReadWriter
	mu   sync.Mutex
}

type websocketCommand struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt,omitempty"`
}

func (s *Server) handleChatWebSocket(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ws, err := upgradeWebSocket(writer, request)
	if err != nil {
		writeJSONError(writer, err, http.StatusBadRequest)
		return
	}
	defer ws.Close()

	sessionID := strings.TrimSpace(request.URL.Query().Get("session_id"))
	var session ChatSession
	if sessionID == "" {
		session = s.chat.Create()
		sessionID = session.ID
	} else {
		existing, ok := s.chat.Get(sessionID)
		if !ok {
			_ = ws.WriteJSON(ChatSocketEnvelope{Type: "chat.error", Error: fmt.Sprintf("chat session %s not found", sessionID)})
			return
		}
		session = existing
	}

	if err := ws.WriteJSON(ChatSocketEnvelope{
		Type:    "session.ready",
		Session: &session,
	}); err != nil {
		return
	}

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()

	if s.eventBus != nil {
		subscription := s.eventBus.Subscribe(64)
		defer s.eventBus.Unsubscribe(subscription.ID)

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-subscription.C:
					if !ok {
						return
					}
					entry := summarizeEvent(event)
					if err := ws.WriteJSON(ChatSocketEnvelope{
						Type:  "audit.event",
						Audit: &entry,
					}); err != nil {
						cancel()
						return
					}
				}
			}
		}()
	}

	for {
		var command websocketCommand
		if err := ws.ReadJSON(&command); err != nil {
			if err == io.EOF {
				return
			}
			_ = ws.WriteJSON(ChatSocketEnvelope{Type: "chat.error", Error: err.Error()})
			return
		}

		switch strings.TrimSpace(command.Type) {
		case "chat.send":
			response, err := s.processChatPrompt(ctx, sessionID, command.Prompt)
			if err != nil {
				_ = ws.WriteJSON(ChatSocketEnvelope{Type: "chat.error", Error: err.Error()})
				continue
			}
			if err := ws.WriteJSON(ChatSocketEnvelope{
				Type:   "chat.result",
				Result: &response,
			}); err != nil {
				return
			}
		case "ping":
			if err := ws.WriteJSON(ChatSocketEnvelope{Type: "pong"}); err != nil {
				return
			}
		default:
			if err := ws.WriteJSON(ChatSocketEnvelope{Type: "chat.error", Error: "unsupported websocket command"}); err != nil {
				return
			}
		}
	}
}

func upgradeWebSocket(writer http.ResponseWriter, request *http.Request) (*websocketConn, error) {
	if !headerContainsToken(request.Header, "Connection", "upgrade") {
		return nil, fmt.Errorf("websocket connection header is required")
	}
	if !headerContainsToken(request.Header, "Upgrade", "websocket") {
		return nil, fmt.Errorf("websocket upgrade header is required")
	}
	if strings.TrimSpace(request.Header.Get("Sec-WebSocket-Version")) != "13" {
		return nil, fmt.Errorf("unsupported websocket version")
	}

	key := strings.TrimSpace(request.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, fmt.Errorf("missing websocket key")
	}

	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("websocket upgrade not supported")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	sum := sha1.Sum([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := rw.WriteString("Upgrade: websocket\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := rw.WriteString("Connection: Upgrade\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := rw.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n\r\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &websocketConn{conn: conn, rw: rw}, nil
}

func (c *websocketConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *websocketConn) ReadJSON(target any) error {
	payload, opcode, err := c.readFrame()
	if err != nil {
		return err
	}
	switch opcode {
	case 0x1:
		return json.Unmarshal(payload, target)
	case 0x8:
		return io.EOF
	case 0x9:
		if err := c.writeFrame(0xA, payload); err != nil {
			return err
		}
		return c.ReadJSON(target)
	case 0xA:
		return c.ReadJSON(target)
	default:
		return fmt.Errorf("unsupported websocket opcode %d", opcode)
	}
}

func (c *websocketConn) WriteJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.writeFrame(0x1, data)
}

func (c *websocketConn) readFrame() ([]byte, byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.rw, header); err != nil {
		return nil, 0, err
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	if !fin {
		return nil, 0, fmt.Errorf("fragmented websocket frames are not supported")
	}

	masked := header[1]&0x80 != 0
	length, err := readPayloadLength(c.rw, header[1]&0x7F)
	if err != nil {
		return nil, 0, err
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.rw, maskKey[:]); err != nil {
			return nil, 0, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.rw, payload); err != nil {
		return nil, 0, err
	}
	if masked {
		for index := range payload {
			payload[index] ^= maskKey[index%4]
		}
	}
	return payload, opcode, nil
}

func (c *websocketConn) writeFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.rw.WriteByte(0x80 | opcode); err != nil {
		return err
	}
	length := len(payload)
	switch {
	case length < 126:
		if err := c.rw.WriteByte(byte(length)); err != nil {
			return err
		}
	case length <= 0xFFFF:
		if err := c.rw.WriteByte(126); err != nil {
			return err
		}
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(length))
		if _, err := c.rw.Write(buf[:]); err != nil {
			return err
		}
	default:
		if err := c.rw.WriteByte(127); err != nil {
			return err
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(length))
		if _, err := c.rw.Write(buf[:]); err != nil {
			return err
		}
	}
	if _, err := c.rw.Write(payload); err != nil {
		return err
	}
	return c.rw.Flush()
}

func readPayloadLength(reader io.Reader, marker byte) (int, error) {
	switch marker {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return 0, err
		}
		return int(binary.BigEndian.Uint16(buf[:])), nil
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return 0, err
		}
		return int(binary.BigEndian.Uint64(buf[:])), nil
	default:
		return int(marker), nil
	}
}

func headerContainsToken(header http.Header, key string, token string) bool {
	for _, value := range header.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
