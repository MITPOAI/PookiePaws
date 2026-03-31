package mcp

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	message := JSONRPCMessage{JSONRPC: "2.0", Method: "ping"}

	if err := writeFrame(&buffer, message); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	got, err := readFrame(bufio.NewReader(bytes.NewReader(buffer.Bytes())))
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if got.Method != "ping" {
		t.Fatalf("unexpected method %q", got.Method)
	}
}

func TestStreamableHTTPTransportJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test" {
			t.Fatalf("missing auth header")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":{"ok":true}}`))
	}))
	defer server.Close()

	transport := NewStreamableHTTPTransport(server.URL, map[string]string{"Authorization": "Bearer test"})
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := transport.Send(context.Background(), JSONRPCMessage{JSONRPC: "2.0", Method: "ping"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := transport.Receive(context.Background()); err != nil {
		t.Fatalf("receive: %v", err)
	}
}

func TestParseSSE(t *testing.T) {
	stream := "data: {\"jsonrpc\":\"2.0\",\"method\":\"ping\"}\n\n"
	count := 0
	err := parseSSE(bytes.NewBufferString(stream), func(message JSONRPCMessage) error {
		count++
		if message.Method != "ping" {
			t.Fatalf("unexpected method %q", message.Method)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parse sse: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one message, got %d", count)
	}
}

func TestDrainStderr(t *testing.T) {
	reader := bytes.NewBufferString("warning\nwarning\n")
	drainStderr(io.NopCloser(reader))
}
