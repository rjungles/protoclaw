package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Transport interface {
	Send(ctx context.Context, req *Request) (*Response, error)
	Close() error
}

type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

func NewStdioTransport(serverCommand string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(serverCommand, args...)

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

func (t *StdioTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := t.stdin.Write([]byte(header)); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := t.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write body: %w", err)
	}

	return t.readResponse(ctx)
}

func (t *StdioTransport) readResponse(ctx context.Context) (*Response, error) {
	done := make(chan struct{})
	var resp *Response
	var err error

	go func() {
		defer close(done)
		resp, err = t.parseResponse()
	}()

	select {
	case <-done:
		return resp, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *StdioTransport) parseResponse() (*Response, error) {
	var contentLength int
	for {
		line, err := t.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			_, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			if err != nil {
				return nil, fmt.Errorf("invalid content length: %w", err)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing content length")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.stdout, body); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	return ParseResponse(body)
}

func (t *StdioTransport) Close() error {
	if err := t.stdin.Close(); err != nil {
		return err
	}
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
		return t.cmd.Wait()
	}
	return nil
}

type SSETransport struct {
	client      *http.Client
	endpoint    string
	headers     map[string]string
	lastEventID string
	mu          sync.Mutex
}

func NewSSETransport(endpoint string, headers map[string]string) *SSETransport {
	return &SSETransport{
		client:   &http.Client{Timeout: 30 * time.Second},
		endpoint: endpoint,
		headers:  headers,
	}
}

func (t *SSETransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	if t.lastEventID != "" {
		httpReq.Header.Set("Last-Event-ID", t.lastEventID)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return ParseResponse(body)
}

func (t *SSETransport) Close() error {
	return nil
}

type HTTPTransport struct {
	client   *http.Client
	endpoint string
	headers  map[string]string
	mu       sync.Mutex
}

func NewHTTPTransport(endpoint string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		client:   &http.Client{Timeout: 30 * time.Second},
		endpoint: endpoint,
		headers:  headers,
	}
}

func (t *HTTPTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return ParseResponse(body)
}

func (t *HTTPTransport) Close() error {
	return nil
}

type MockTransport struct {
	responses map[int64]*Response
	tools     []Tool
	resources []Resource
	prompts   []Prompt
	mu        sync.Mutex
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		responses: make(map[int64]*Response),
		tools: []Tool{
			{Name: "test_tool", Description: "A test tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		resources: []Resource{
			{URI: "test://resource", Name: "Test Resource"},
		},
		prompts: []Prompt{
			{Name: "test_prompt", Description: "A test prompt"},
		},
	}
}

func (t *MockTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if resp, ok := t.responses[req.ID]; ok {
		return resp, nil
	}

	switch req.Method {
	case "initialize":
		result := InitializeResult{
			ProtocolVersion: ProtocolVersion,
			ServerInfo: ServerInfo{
				Name:    "mock-server",
				Version: "1.0.0",
			},
			Capabilities: ServerCapabilities{
				Tools:     &ToolsCapability{},
				Resources: &ResourcesCapability{},
				Prompts:   &PromptsCapability{},
			},
		}
		data, _ := json.Marshal(result)
		return &Response{JSONRPC: JSONRPCVersion, ID: req.ID, Result: data}, nil

	case "tools/list":
		result := ListToolsResult{Tools: t.tools}
		data, _ := json.Marshal(result)
		return &Response{JSONRPC: JSONRPCVersion, ID: req.ID, Result: data}, nil

	case "tools/call":
		result := CallToolResult{
			Content: []Content{{Type: "text", Text: "mock tool result"}},
		}
		data, _ := json.Marshal(result)
		return &Response{JSONRPC: JSONRPCVersion, ID: req.ID, Result: data}, nil

	case "resources/list":
		result := ListResourcesResult{Resources: t.resources}
		data, _ := json.Marshal(result)
		return &Response{JSONRPC: JSONRPCVersion, ID: req.ID, Result: data}, nil

	case "prompts/list":
		result := ListPromptsResult{Prompts: t.prompts}
		data, _ := json.Marshal(result)
		return &Response{JSONRPC: JSONRPCVersion, ID: req.ID, Result: data}, nil

	default:
		return &Response{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error:   &Error{Code: -32601, Message: "Method not found"},
		}, nil
	}
}

func (t *MockTransport) SetResponse(id int64, resp *Response) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responses[id] = resp
}

func (t *MockTransport) SetTools(tools []Tool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tools = tools
}

func (t *MockTransport) Close() error {
	return nil
}
