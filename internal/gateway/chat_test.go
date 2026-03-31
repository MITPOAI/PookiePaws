package gateway

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGatewayChatSessionLifecycle(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/chat/sessions", nil)
	createRec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d body=%s", createRec.Code, createRec.Body.String())
	}

	var session ChatSession
	if err := json.Unmarshal(createRec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/api/v1/chat/sessions/"+session.ID+"/messages", strings.NewReader(`{"prompt":"validate this url"}`))
	messageReq.Header.Set("Content-Type", "application/json")
	messageRec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", messageRec.Code, messageRec.Body.String())
	}

	var response ChatDispatchResponse
	if err := json.Unmarshal(messageRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.UserMessage.Role != "user" {
		t.Fatalf("expected user role, got %q", response.UserMessage.Role)
	}
	if response.AssistantMessage.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", response.AssistantMessage.Role)
	}
	if len(response.Steps) < 2 {
		t.Fatalf("expected chat steps, got %d", len(response.Steps))
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/chat/sessions/"+session.ID+"/messages", nil)
	listRec := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", listRec.Code, listRec.Body.String())
	}

	var messages []ChatMessage
	if err := json.Unmarshal(listRec.Body.Bytes(), &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestGatewayChatWebSocket(t *testing.T) {
	h := newHarness(t, "127.0.0.1:18800", stubBrain{})
	httpServer := httptest.NewServer(h.server.Handler())
	defer httpServer.Close()

	ws, err := dialTestWebSocket(httpServer.URL + "/api/v1/chat/ws")
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer ws.Close()

	var ready ChatSocketEnvelope
	if err := ws.ReadJSON(&ready); err != nil {
		t.Fatalf("read ready envelope: %v", err)
	}
	if ready.Type != "session.ready" {
		t.Fatalf("unexpected ready type %q", ready.Type)
	}
	if ready.Session == nil || ready.Session.ID == "" {
		t.Fatalf("expected session payload")
	}

	if err := ws.WriteJSON(websocketCommand{
		Type:   "chat.send",
		Prompt: "validate this url",
	}); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	for {
		var envelope ChatSocketEnvelope
		if err := ws.ReadJSON(&envelope); err != nil {
			t.Fatalf("read envelope: %v", err)
		}
		if envelope.Type != "chat.result" {
			continue
		}
		if envelope.Result == nil {
			t.Fatalf("expected chat result payload")
		}
		if envelope.Result.UserMessage.Role != "user" {
			t.Fatalf("expected user role, got %q", envelope.Result.UserMessage.Role)
		}
		if envelope.Result.AssistantMessage.Role != "assistant" {
			t.Fatalf("expected assistant role, got %q", envelope.Result.AssistantMessage.Role)
		}
		return
	}
}

type testWebSocket struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func dialTestWebSocket(rawURL string) (*testWebSocket, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		return nil, err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n\r\n", path, parsed.Host, key)
	if _, err := io.WriteString(conn, request); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected upgrade response %q", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if line == "\r\n" {
			break
		}
	}

	return &testWebSocket{
		conn:   conn,
		reader: reader,
		writer: bufio.NewWriter(conn),
	}, nil
}

func (w *testWebSocket) Close() error {
	if w == nil || w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

func (w *testWebSocket) WriteJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	maskKey := [4]byte{}
	if _, err := rand.Read(maskKey[:]); err != nil {
		return err
	}
	for index := range data {
		data[index] ^= maskKey[index%4]
	}

	if err := w.writer.WriteByte(0x81); err != nil {
		return err
	}
	length := len(data)
	switch {
	case length < 126:
		if err := w.writer.WriteByte(0x80 | byte(length)); err != nil {
			return err
		}
	case length <= 0xFFFF:
		if err := w.writer.WriteByte(0x80 | 126); err != nil {
			return err
		}
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(length))
		if _, err := w.writer.Write(buf[:]); err != nil {
			return err
		}
	default:
		if err := w.writer.WriteByte(0x80 | 127); err != nil {
			return err
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(length))
		if _, err := w.writer.Write(buf[:]); err != nil {
			return err
		}
	}
	if _, err := w.writer.Write(maskKey[:]); err != nil {
		return err
	}
	if _, err := w.writer.Write(data); err != nil {
		return err
	}
	return w.writer.Flush()
}

func (w *testWebSocket) ReadJSON(target any) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(w.reader, header); err != nil {
		return err
	}

	opcode := header[0] & 0x0F
	length, err := testReadPayloadLength(w.reader, header[1]&0x7F)
	if err != nil {
		return err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(w.reader, payload); err != nil {
		return err
	}
	if opcode == 0x8 {
		return io.EOF
	}
	return json.Unmarshal(payload, target)
}

func testReadPayloadLength(reader io.Reader, marker byte) (int, error) {
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
