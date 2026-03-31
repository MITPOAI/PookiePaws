package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type StdioTransport struct {
	Command     string
	Args        []string
	Env         []string
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	closed bool
}

func NewStdioTransport(command string, args ...string) *StdioTransport {
	return &StdioTransport{Command: command, Args: args}
}

func (t *StdioTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stdin != nil && t.stdout != nil {
		return nil
	}
	if t.Command == "" {
		return fmt.Errorf("stdio command is required")
	}

	run := t.execCommand
	if run == nil {
		run = exec.CommandContext
	}

	cmd := run(ctx, t.Command, t.Args...)
	if len(t.Env) > 0 {
		cmd.Env = append(cmd.Env, t.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	go drainStderr(stderr)

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = bufio.NewReader(stdout)
	return nil
}

func (t *StdioTransport) Send(_ context.Context, message JSONRPCMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return io.ErrClosedPipe
	}
	if t.stdin == nil {
		return fmt.Errorf("stdio transport is not connected")
	}
	return writeFrame(t.stdin, message)
}

func (t *StdioTransport) Receive(_ context.Context) (JSONRPCMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return JSONRPCMessage{}, io.ErrClosedPipe
	}
	if t.stdout == nil {
		return JSONRPCMessage{}, fmt.Errorf("stdio transport is not connected")
	}
	return readFrame(t.stdout)
}

func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

type StreamableHTTPTransport struct {
	BaseURL string
	Client  *http.Client
	Headers map[string]string

	mu     sync.Mutex
	queue  chan JSONRPCMessage
	closed bool
}

func NewStreamableHTTPTransport(baseURL string, headers map[string]string) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		BaseURL: baseURL,
		Headers: headers,
	}
}

func (t *StreamableHTTPTransport) Connect(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.queue == nil {
		t.queue = make(chan JSONRPCMessage, 32)
	}
	return nil
}

func (t *StreamableHTTPTransport) Send(ctx context.Context, message JSONRPCMessage) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	queue := t.queue
	client := t.Client
	headers := t.Headers
	baseURL := t.BaseURL
	t.mu.Unlock()

	if queue == nil {
		return fmt.Errorf("http transport is not connected")
	}
	if client == nil {
		client = http.DefaultClient
	}

	body, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return parseSSE(resp.Body, func(message JSONRPCMessage) error {
			select {
			case queue <- message:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	var rpc JSONRPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return err
	}
	select {
	case queue <- rpc:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *StreamableHTTPTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	t.mu.Lock()
	queue := t.queue
	closed := t.closed
	t.mu.Unlock()

	if closed {
		return JSONRPCMessage{}, io.ErrClosedPipe
	}
	if queue == nil {
		return JSONRPCMessage{}, fmt.Errorf("http transport is not connected")
	}

	select {
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case message, ok := <-queue:
		if !ok {
			return JSONRPCMessage{}, io.EOF
		}
		return message, nil
	}
}

func (t *StreamableHTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true
	if t.queue != nil {
		close(t.queue)
	}
	return nil
}

func writeFrame(writer io.Writer, message JSONRPCMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = writer.Write(body)
	return err
}

func readFrame(reader *bufio.Reader) (JSONRPCMessage, error) {
	length, err := readContentLength(reader)
	if err != nil {
		return JSONRPCMessage{}, err
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return JSONRPCMessage{}, err
	}

	var message JSONRPCMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return JSONRPCMessage{}, err
	}
	return message, nil
}

func readContentLength(reader *bufio.Reader) (int, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "content-length:") {
			return 0, fmt.Errorf("unexpected frame header %q", line)
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
		if value == line {
			value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
		}
		if _, err := reader.ReadString('\n'); err != nil {
			return 0, err
		}
		length, err := strconv.Atoi(value)
		if err != nil {
			return 0, err
		}
		return length, nil
	}
}

func parseSSE(reader io.Reader, deliver func(JSONRPCMessage) error) error {
	scanner := bufio.NewScanner(reader)
	var data strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			var message JSONRPCMessage
			if err := json.Unmarshal([]byte(data.String()), &message); err != nil {
				return err
			}
			if err := deliver(message); err != nil {
				return err
			}
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	if data.Len() > 0 {
		var message JSONRPCMessage
		if err := json.Unmarshal([]byte(data.String()), &message); err != nil {
			return err
		}
		return deliver(message)
	}
	return nil
}

func drainStderr(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}
