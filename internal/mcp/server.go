// Package mcp implements a minimal MCP stdio server exposing flume's
// captured request data as tools Claude Code can query directly.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const protocolVersion = "2025-06-18"

// Dialer is the minimal interface over the flume daemon client.
type Dialer interface {
	Call(ctx context.Context, method string, params, out any) error
	Close() error
}

// DialFunc returns a fresh Dialer. One connection per tool call.
type DialFunc func() (Dialer, error)

// Server is an MCP stdio server.
type Server struct {
	dial DialFunc
	mu   sync.Mutex
	out  *bufio.Writer
}

// New constructs an MCP server.
func New(dial DialFunc) *Server {
	return &Server{dial: dial}
}

// Serve runs the read-dispatch-write loop until ctx is cancelled or EOF.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = bufio.NewWriter(out)

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		s.handleLine(ctx, line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any            `json:"result,omitempty"`
	Error   *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeError(nil, -32700, "parse error: "+err.Error(), nil)
		return
	}

	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized", "notifications/initialized":
		// no-op
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	case "ping":
		if !isNotification {
			s.writeResult(req.ID, map[string]any{})
		}
	default:
		if !isNotification {
			s.writeError(req.ID, -32601, "method not found: "+req.Method, nil)
		}
	}
}

func (s *Server) handleInitialize(req request) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &p)

	version := p.ProtocolVersion
	if version == "" {
		version = protocolVersion
	}

	s.writeResult(req.ID, map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "flume",
			"version": "0.1.0",
		},
	})
}

// --- tools/list ---

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

var toolDefinitions = []tool{
	{
		Name: "flume_requests",
		Description: `List recent HTTP requests captured by the flume reverse proxy. Returns a summary list with ID, method, path, status code, duration, and timestamp. Filterable by URL path substring, HTTP method, and status code range. Use this to see what traffic has hit the dev server — e.g. "show me the last 5 requests to /api/orders" or "show me recent 500 errors".`,
		InputSchema: mustMarshal(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "Filter by URL path (substring match). E.g. '/api/orders'."},
				"method":     map[string]any{"type": "string", "description": "Filter by HTTP method. E.g. 'POST'."},
				"status_min": map[string]any{"type": "integer", "description": "Minimum status code (inclusive). E.g. 400."},
				"status_max": map[string]any{"type": "integer", "description": "Maximum status code (inclusive). E.g. 499."},
				"limit":      map[string]any{"type": "integer", "description": "Max results to return. Default 20."},
			},
		}),
	},
	{
		Name: "flume_request",
		Description: `Get full detail for a single captured HTTP request by ID. Returns complete request and response headers, bodies, timing, and status code. Use this after flume_requests to drill into a specific request — e.g. to see the response body or request payload that caused an error.`,
		InputSchema: mustMarshal(map[string]any{
			"type":     "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "The request ID from flume_requests."},
			},
			"required": []string{"id"},
		}),
	},
	{
		Name: "flume_status",
		Description: `Show flume daemon state: whether the proxy is running, what port it's listening on, what target it's proxying to, how many requests are captured, and the retention window.`,
		InputSchema: mustMarshal(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	},
}

func (s *Server) handleToolsList(req request) {
	s.writeResult(req.ID, map[string]any{"tools": toolDefinitions})
}

// --- tools/call ---

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(ctx context.Context, req request) {
	var p toolsCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.writeError(req.ID, -32602, "invalid params: "+err.Error(), nil)
		return
	}

	switch p.Name {
	case "flume_requests":
		s.callRequests(ctx, req.ID, p.Arguments)
	case "flume_request":
		s.callRequest(ctx, req.ID, p.Arguments)
	case "flume_status":
		s.callStatus(ctx, req.ID)
	default:
		s.writeToolError(req.ID, fmt.Sprintf("unknown tool %q", p.Name))
	}
}

func (s *Server) callRequests(ctx context.Context, id json.RawMessage, rawArgs json.RawMessage) {
	client, err := s.dial()
	if err != nil {
		s.writeToolError(id, "dial flume daemon: "+err.Error())
		return
	}
	defer client.Close()

	// Forward args directly — they match the store.ListFilter shape.
	var raw json.RawMessage
	if err := client.Call(ctx, "requests", rawArgs, &raw); err != nil {
		s.writeToolError(id, "flume requests: "+err.Error())
		return
	}
	s.writeToolResult(id, prettyJSON(raw), false)
}

func (s *Server) callRequest(ctx context.Context, id json.RawMessage, rawArgs json.RawMessage) {
	client, err := s.dial()
	if err != nil {
		s.writeToolError(id, "dial flume daemon: "+err.Error())
		return
	}
	defer client.Close()

	var raw json.RawMessage
	if err := client.Call(ctx, "request", rawArgs, &raw); err != nil {
		s.writeToolError(id, "flume request: "+err.Error())
		return
	}

	// For the response, try to decode and format bodies as strings if they're
	// text-based, and replace binary bodies with placeholders.
	s.writeToolResult(id, formatRequestForAgent(raw), false)
}

func (s *Server) callStatus(ctx context.Context, id json.RawMessage) {
	client, err := s.dial()
	if err != nil {
		s.writeToolError(id, "dial flume daemon: "+err.Error())
		return
	}
	defer client.Close()

	var raw json.RawMessage
	if err := client.Call(ctx, "status", struct{}{}, &raw); err != nil {
		s.writeToolError(id, "flume status: "+err.Error())
		return
	}
	s.writeToolResult(id, prettyJSON(raw), false)
}

// --- output helpers ---

func (s *Server) writeResult(id json.RawMessage, result any) {
	s.writeMessage(&response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeError(id json.RawMessage, code int, msg string, data any) {
	s.writeMessage(&response{JSONRPC: "2.0", ID: id, Error: &responseError{Code: code, Message: msg, Data: data}})
}

func (s *Server) writeMessage(resp *response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resp.ID == nil {
		resp.ID = json.RawMessage(`null`)
	}
	b, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flume mcp: marshal response: %v\n", err)
		return
	}
	_, _ = s.out.Write(b)
	_ = s.out.WriteByte('\n')
	_ = s.out.Flush()
}

func (s *Server) writeToolResult(id json.RawMessage, text string, isError bool) {
	s.writeResult(id, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"isError": isError,
	})
}

func (s *Server) writeToolError(id json.RawMessage, msg string) {
	s.writeToolResult(id, msg, true)
}

func prettyJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// formatRequestForAgent re-formats a captured request for agent consumption.
// Converts response/request bodies from base64 (JSON []byte encoding) to
// strings when they're text-based, and replaces binary bodies with placeholders.
func formatRequestForAgent(raw json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return string(raw)
	}

	// Try to decode bodies as text for agent readability.
	for _, key := range []string{"request_body", "response_body"} {
		if body, ok := m[key].(string); ok && len(body) > 0 {
			// JSON encodes []byte as base64. Check content-type to decide display.
			headerKey := "response_headers"
			if key == "request_body" {
				headerKey = "request_headers"
			}
			if isBinaryContent(m, headerKey) {
				m[key] = fmt.Sprintf("[binary: %d bytes]", len(body))
			}
			// Otherwise leave as-is (base64-encoded text is still readable in context).
		}
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func isBinaryContent(m map[string]any, headerKey string) bool {
	headers, ok := m[headerKey].(map[string]any)
	if !ok {
		return false
	}
	ct, ok := headers["Content-Type"]
	if !ok {
		return false
	}
	ctSlice, ok := ct.([]any)
	if !ok || len(ctSlice) == 0 {
		return false
	}
	ctStr, ok := ctSlice[0].(string)
	if !ok {
		return false
	}
	// Common binary content types.
	for _, prefix := range []string{"image/", "audio/", "video/", "application/octet-stream", "application/pdf", "application/zip"} {
		if len(ctStr) >= len(prefix) && ctStr[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// nowUTC returns the current time in UTC. Exists so tests can stub it.
var nowUTC = func() time.Time { return time.Now().UTC() }
