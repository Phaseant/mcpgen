package generate

import (
	"bytes"
	"fmt"
)

func (m *model) runtime() []byte {
	var b bytes.Buffer
	b.WriteString(`import (
"context"
"encoding/json"
"errors"
"github.com/modelcontextprotocol/go-sdk/mcp"
"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

var ErrNotImplemented = ToolError{Code: "not_implemented", Message: "tool not implemented"}
type ToolError struct { Code string ` + "`json:\"code\"`" + `; Message string ` + "`json:\"message\"`" + ` }
func (e ToolError) Error() string { return e.Code + ": " + e.Message }
func NewToolError(code, message string) ToolError { return ToolError{Code: code, Message: message} }

// ToolHandlerFunc receives a tool name and its decoded generated input type.
// Return the tool's generated output type on success.
type ToolHandlerFunc func(context.Context, string, any) (any, error)
type Middleware func(ToolHandlerFunc) ToolHandlerFunc

func wrapTool(next ToolHandlerFunc, middleware []Middleware) ToolHandlerFunc {
for i := len(middleware)-1; i >= 0; i-- {
next = middleware[i](next)
if next == nil { return func(context.Context, string, any) (any, error) { return nil, errors.New("middleware returned nil handler") } }
}
return next
}

func adaptError(err error) error {
var value ToolError
if errors.As(err, &value) { data, _ := json.Marshal(value); return errors.New(string(data)) }
var ptr *ToolError
if errors.As(err, &ptr) && ptr != nil { data, _ := json.Marshal(ptr); return errors.New(string(data)) }
return &jsonrpc.Error{Code: -32603, Message: "internal error"}
}
`)
	for _, t := range m.Tools {
		fmt.Fprintf(&b, `func register%s(server *mcp.Server, handler Handler, middleware []Middleware) {
call := wrapTool(func(ctx context.Context, _ string, input any) (any, error) {
typed, ok := input.(%s)
if !ok { return nil, errors.New("middleware returned invalid input type") }
return handler.%s(ctx, typed)
}, middleware)
mcp.AddTool(server, tool%s, func(ctx context.Context, _ *mcp.CallToolRequest, input %s) (*mcp.CallToolResult, %s, error) {
var zero %s
value, err := call(ctx, %q, input)
if err != nil { return nil, zero, adaptError(err) }
output, ok := value.(%s)
if !ok { return nil, zero, adaptError(errors.New("middleware returned invalid output type")) }
return nil, output, nil
})
}
`, t.Method, t.Input, t.Method, t.Method, t.Input, t.Output, t.Output, t.Name, t.Output)
	}
	return b.Bytes()
}
