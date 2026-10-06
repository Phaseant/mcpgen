package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/phaseant/mcpgen/example/internal/mcpapi"
	"github.com/phaseant/mcpgen/example/internal/service"
)

func session(t *testing.T, handler mcpapi.Handler) (context.Context, *mcp.ClientSession) {
	t.Helper()
	server, err := mcpapi.NewServer(handler)
	if err != nil {
		t.Fatal(err)
	}
	return connect(t, server.MCP())
}

func connect(t *testing.T, server *mcp.Server) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return ctx, cs
}

func TestEchoProtocol(t *testing.T) {
	ctx, cs := session(t, New(&service.Pets{}))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 4 || list.Tools[1].Name != "echo" {
		t.Fatalf("tools: %+v", list.Tools)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output mcpapi.EchoOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if res.IsError || output.Message != "hello" || len(res.Content) != 1 {
		t.Fatalf("result: %+v, output: %+v", res, output)
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
	if err != nil || !res.IsError {
		t.Fatalf("missing required argument: result=%+v error=%v", res, err)
	}
}

func TestPetsProtocol(t *testing.T) {
	ctx, cs := session(t, New(&service.Pets{}))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var get *mcp.Tool
	for _, tool := range list.Tools {
		if tool.Name == "get_pet" {
			get = tool
		}
	}
	if get == nil || get.Title != "Get pet" || get.Annotations == nil || !get.Annotations.ReadOnlyHint || get.Annotations.DestructiveHint == nil || *get.Annotations.DestructiveHint || get.Annotations.OpenWorldHint == nil || *get.Annotations.OpenWorldHint {
		t.Fatalf("get_pet metadata: %+v", get)
	}
	created, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_pet", Arguments: map[string]any{"name": "Mittens", "kind": "cat"}})
	if err != nil || created.IsError {
		t.Fatalf("create result=%+v err=%v", created, err)
	}
	data, err := json.Marshal(created.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var pet mcpapi.Pet
	if err := json.Unmarshal(data, &pet); err != nil {
		t.Fatal(err)
	}
	if pet.ID == "" || pet.Name != "Mittens" || pet.Kind != mcpapi.PetKindCat {
		t.Fatalf("created pet=%+v", pet)
	}
	got, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_pet", Arguments: map[string]any{"pet_id": pet.ID}})
	if err != nil || got.IsError {
		t.Fatalf("get result=%+v err=%v", got, err)
	}
	gotData, err := json.Marshal(got.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(gotData) {
		t.Fatalf("get=%s create=%s", gotData, data)
	}
	searched, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": "mittens", "limit": 1, "sort": "created_at", "filter": map[string]any{"ids": []string{pet.ID}}}})
	if err != nil || searched.IsError {
		t.Fatalf("search result=%+v err=%v", searched, err)
	}
	searchData, err := json.Marshal(searched.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output mcpapi.SearchResult
	if err := json.Unmarshal(searchData, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Pets) != 1 || output.Pets[0] != pet {
		t.Fatalf("search output=%+v", output)
	}
	dogResult, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_pet", Arguments: map[string]any{"name": "Rover", "kind": "dog"}})
	if err != nil || dogResult.IsError {
		t.Fatalf("create dog result=%+v err=%v", dogResult, err)
	}
	dogData, err := json.Marshal(dogResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var dog mcpapi.Pet
	if err := json.Unmarshal(dogData, &dog); err != nil {
		t.Fatal(err)
	}
	if dog.Kind != mcpapi.PetKindDog || dog.Name != "Rover" || dog.ID == pet.ID {
		t.Fatalf("dog=%+v", dog)
	}
	dogResult, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_pet", Arguments: map[string]any{"pet_id": dog.ID}})
	if err != nil || dogResult.IsError {
		t.Fatalf("get dog result=%+v err=%v", dogResult, err)
	}
	gotDog, err := json.Marshal(dogResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotDog) != string(dogData) {
		t.Fatalf("get dog=%s create dog=%s", gotDog, dogData)
	}
	all, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": ""}})
	if err != nil || all.IsError {
		t.Fatalf("search pets result=%+v err=%v", all, err)
	}
	allData, err := json.Marshal(all.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(allData, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Pets) != 2 || output.Pets[0] != pet || output.Pets[1] != dog {
		t.Fatalf("cats and dogs=%+v", output)
	}
	missing, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_pet", Arguments: map[string]any{"pet_id": "999"}})
	if err != nil || !missing.IsError {
		t.Fatalf("missing result=%+v err=%v", missing, err)
	}
	text := missing.Content[0].(*mcp.TextContent).Text
	var expected mcpapi.ToolError
	if err := json.Unmarshal([]byte(text), &expected); err != nil {
		t.Fatal(err)
	}
	if expected.Code != "pet_not_found" || expected.Message != "pet does not exist" {
		t.Fatalf("tool error=%+v", expected)
	}
}

type observingHandler struct {
	mcpapi.UnimplementedHandler
	calls     atomic.Int64
	input     mcpapi.SearchInput
	err       error
	badOutput bool
}

func (h *observingHandler) Search(_ context.Context, input mcpapi.SearchInput) (mcpapi.SearchResult, error) {
	h.calls.Add(1)
	h.input = input
	return mcpapi.SearchResult{Pets: []mcpapi.Pet{}}, nil
}

func (h *observingHandler) Echo(_ context.Context, input mcpapi.EchoInput) (mcpapi.EchoOutput, error) {
	h.calls.Add(1)
	if h.badOutput {
		return mcpapi.EchoOutput{}, nil
	}
	return mcpapi.EchoOutput{Message: input.Message}, h.err
}

func TestInputValidationBeforeDispatch(t *testing.T) {
	h := &observingHandler{}
	ctx, cs := session(t, h)
	cases := []struct {
		name, tool string
		args       map[string]any
	}{
		{"missing required", "echo", map[string]any{}},
		{"wrong type", "echo", map[string]any{"message": 42}},
		{"length", "echo", map[string]any{"message": ""}},
		{"minimum", "search", map[string]any{"query": "", "limit": 0}},
		{"maximum", "search", map[string]any{"query": "", "limit": 101}},
		{"integer", "search", map[string]any{"query": "", "limit": 1.5}},
		{"enum", "search", map[string]any{"query": "", "sort": "unknown"}},
		{"extra property", "search", map[string]any{"query": "", "unknown": true}},
		{"nested extra property", "search", map[string]any{"query": "", "filter": map[string]any{"unknown": true}}},
		{"null optional", "search", map[string]any{"query": "", "limit": nil}},
		{"unique items", "search", map[string]any{"query": "", "filter": map[string]any{"ids": []string{"1", "1"}}}},
		{"pattern", "get_pet", map[string]any{"pet_id": "invalid"}},
		{"pet kind", "create_pet", map[string]any{"name": "Bird", "kind": "bird"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil || !res.IsError {
				t.Fatalf("result=%+v err=%v", res, err)
			}
			if len(res.Content) == 0 || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "validating") {
				t.Fatalf("expected validation failure: %+v", res)
			}
			if h.calls.Load() != 0 {
				t.Fatal("invalid input reached handler")
			}
		})
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": ""}})
	if err != nil || res.IsError {
		t.Fatalf("optional fields result=%+v err=%v", res, err)
	}
	if h.calls.Load() != 1 || h.input.Limit != nil || h.input.Sort != nil || h.input.Filter != nil {
		t.Fatalf("optional input=%+v calls=%d", h.input, h.calls.Load())
	}
}

func TestOutputValidation(t *testing.T) {
	ctx, cs := session(t, &observingHandler{badOutput: true})
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "hello"}})
	if err == nil {
		t.Fatalf("invalid output was accepted: %+v", result)
	}
}

func TestHandlerErrorAdaptation(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		protocol bool
		want     string
	}{
		{"unexpected", errors.New("database password secret"), true, "internal error"},
		{"wrapped expected", fmt.Errorf("wrapped: %w", mcpapi.NewToolError("not_found", "missing")), false, "not_found"},
		{"pointer expected", &mcpapi.ToolError{Code: "not_found", Message: "missing"}, false, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cs := session(t, &observingHandler{err: tc.err})
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "hello"}})
			if tc.protocol {
				if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
					t.Fatalf("result=%+v err=%v", res, err)
				}
			} else if err != nil || !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, tc.want) {
				t.Fatalf("result=%+v err=%v", res, err)
			}
		})
	}
}

func TestNilHandler(t *testing.T) {
	var typed *Handler
	for _, handler := range []mcpapi.Handler{nil, typed} {
		if _, err := mcpapi.NewServer(handler); err == nil {
			t.Fatal("nil handler accepted")
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type userKey struct{}

func TestExistingHTTPServer(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts *mcpapi.HTTPOptions
	}{{"stateful SSE", nil}, {"stateless JSON", &mcpapi.HTTPOptions{Stateless: true, JSONResponse: true}}} {
		t.Run(mode.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var calls atomic.Int64
			mw := func(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
				return func(ctx context.Context, tool string, input any) (any, error) {
					if ctx.Value(userKey{}) != "alice" {
						return nil, mcpapi.NewToolError("unauthorized", "missing HTTP context")
					}
					if tool != "echo" {
						return nil, errors.New("unexpected tool")
					}
					if _, ok := input.(mcpapi.EchoInput); !ok {
						return nil, errors.New("input was not decoded")
					}
					calls.Add(1)
					return next(ctx, tool, input)
				}
			}
			server, err := mcpapi.NewServer(New(&service.Pets{}), mcpapi.WithMiddleware(mw))
			if err != nil {
				t.Fatal(err)
			}
			endpoint := server.HTTPHandler(mode.opts)
			mux := http.NewServeMux()
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
			mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				endpoint.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, "alice")))
			}))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/mcp", nil))
			if recorder.Code != http.StatusUnauthorized || calls.Load() != 0 {
				t.Fatalf("unauthorized status=%d calls=%d", recorder.Code, calls.Load())
			}
			health := httptest.NewRecorder()
			mux.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
			if health.Code != http.StatusNoContent {
				t.Fatalf("health status=%d", health.Code)
			}
			host := httptest.NewServer(mux)
			defer host.Close()
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				copy := r.Clone(r.Context())
				copy.Header.Set("Authorization", "Bearer test-token")
				return host.Client().Transport.RoundTrip(copy)
			})}
			client := mcp.NewClient(&mcp.Implementation{Name: "http-test", Version: "1"}, nil)
			cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: host.URL + "/mcp", HTTPClient: httpClient}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			list, err := cs.ListTools(ctx, nil)
			if err != nil || len(list.Tools) != 4 {
				t.Fatalf("tools=%+v error=%v", list, err)
			}
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "mounted"}})
			if err != nil || res.IsError || calls.Load() != 1 {
				t.Fatalf("result=%+v error=%v calls=%d", res, err, calls.Load())
			}
			data, err := json.Marshal(res.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != `{"message":"mounted"}` {
				t.Fatalf("output=%s", data)
			}
			res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
			if err != nil || !res.IsError || calls.Load() != 1 {
				t.Fatalf("invalid result=%+v error=%v calls=%d", res, err, calls.Load())
			}
		})
	}
}

func TestRegisterToolsOnExistingMCPServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "existing", Version: "1"}, nil)
	var protocolCalls atomic.Int64
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				protocolCalls.Add(1)
			}
			return next(ctx, method, req)
		}
	})
	mcp.AddTool(server, &mcp.Tool{Name: "legacy"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	var calls atomic.Int64
	mw := func(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
		return func(ctx context.Context, tool string, input any) (any, error) {
			calls.Add(1)
			return next(ctx, tool, input)
		}
	}
	if err := mcpapi.RegisterTools(server, New(&service.Pets{}), mw); err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server)
	if cs.InitializeResult().ServerInfo.Name != "existing" {
		t.Fatal("existing server metadata changed")
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 5 {
		t.Fatalf("tools=%+v err=%v", list, err)
	}
	for _, name := range []string{"legacy", "echo"} {
		args := map[string]any{}
		if name == "echo" {
			args["message"] = "existing"
		}
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("tool=%s result=%+v err=%v", name, res, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("middleware calls=%d", calls.Load())
	}
	if protocolCalls.Load() != 2 {
		t.Fatalf("SDK middleware calls=%d", protocolCalls.Load())
	}
}

func TestToolMiddlewareOrder(t *testing.T) {
	var mu sync.Mutex
	var events []string
	record := func(event string) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }
	makeMiddleware := func(name string) mcpapi.Middleware {
		return func(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
			return func(ctx context.Context, tool string, input any) (any, error) {
				record(name + " before")
				output, err := next(ctx, tool, input)
				record(name + " after")
				return output, err
			}
		}
	}
	server, err := mcpapi.NewServer(New(&service.Pets{}), mcpapi.WithMiddleware(makeMiddleware("first")), mcpapi.WithMiddleware(makeMiddleware("second")))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs := connect(t, server.MCP())
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "ordered"}})
	if err != nil || res.IsError {
		t.Fatalf("result=%+v err=%v", res, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(events, []string{"first before", "second before", "second after", "first after"}) {
		t.Fatalf("events=%v", events)
	}
}

func TestMiddlewareFailures(t *testing.T) {
	cases := []struct {
		name       string
		middleware mcpapi.Middleware
		expected   bool
	}{
		{"short circuit", func(mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
			return func(context.Context, string, any) (any, error) {
				return nil, mcpapi.NewToolError("forbidden", "blocked")
			}
		}, true},
		{"wrong input", func(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
			return func(ctx context.Context, tool string, input any) (any, error) { return next(ctx, tool, "wrong") }
		}, false},
		{"wrong output", func(mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
			return func(context.Context, string, any) (any, error) { return "wrong", nil }
		}, false},
		{"nil next", func(mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc { return nil }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &observingHandler{}
			outer := func(next mcpapi.ToolHandlerFunc) mcpapi.ToolHandlerFunc {
				return func(ctx context.Context, tool string, input any) (any, error) { return next(ctx, tool, input) }
			}
			server, err := mcpapi.NewServer(handler, mcpapi.WithMiddleware(outer, tc.middleware))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cs := connect(t, server.MCP())
			res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "blocked"}})
			if tc.expected {
				if err != nil || !res.IsError {
					t.Fatalf("result=%+v err=%v", res, err)
				}
			} else if err == nil {
				t.Fatalf("invalid middleware succeeded: %+v", res)
			}
			if handler.calls.Load() != 0 {
				t.Fatal("handler was invoked")
			}
		})
	}
	if _, err := mcpapi.NewServer(&observingHandler{}, mcpapi.WithMiddleware(nil)); err == nil {
		t.Fatal("nil middleware accepted")
	}
	if err := mcpapi.RegisterTools(nil, &observingHandler{}); err == nil {
		t.Fatal("nil SDK server accepted")
	}
}
