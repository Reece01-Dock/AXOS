// Package mcp implements a minimal MCP (Model Context Protocol) server:
// JSON-RPC 2.0 over a byte stream (stdio in practice — see
// docs/architecture.md "MCP transport"). It exposes the RouterBackend
// methods, plus rollback and audit, as MCP tools per docs/mcp-api.md.
package mcp

import "encoding/json"

// JSON-RPC 2.0 envelope types.

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes plus an AXOS-specific range.
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603

	// ErrRollbackRequired signals a [danger] tool was called with no armed
	// rollback transaction (docs/mcp-api.md "Danger enforcement").
	ErrRollbackRequired = -32001
	// ErrRollbackBusy signals rollback.arm was called while a transaction is
	// already pending.
	ErrRollbackBusy = -32002
)

// Tool describes one callable MCP tool, as returned by tools/list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsListResult is the result of a tools/list call.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams is the params of a tools/call request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolContent is one piece of content in a tool call result (MCP supports
// multiple content blocks; AXOS only emits a single JSON text block today).
type ToolContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// ToolCallResult is the result of a tools/call request.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// InitializeResult is returned in response to the "initialize" method.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools struct{} `json:"tools"`
}
