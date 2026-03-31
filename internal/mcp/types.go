package mcp

import (
	"context"
	"encoding/json"
)

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type Transport interface {
	Connect(ctx context.Context) error
	Send(ctx context.Context, message JSONRPCMessage) error
	Receive(ctx context.Context) (JSONRPCMessage, error)
	Close() error
}

type Client struct {
	transport Transport
}

func NewClient(transport Transport) *Client {
	return &Client{transport: transport}
}

func (c *Client) Connect(ctx context.Context) error {
	return c.transport.Connect(ctx)
}

func (c *Client) Send(ctx context.Context, message JSONRPCMessage) error {
	return c.transport.Send(ctx, message)
}

func (c *Client) Receive(ctx context.Context) (JSONRPCMessage, error) {
	return c.transport.Receive(ctx)
}

func (c *Client) Close() error {
	return c.transport.Close()
}
