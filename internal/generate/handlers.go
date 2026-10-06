package generate

import (
	"bytes"
	"fmt"
)

func (m *model) handlers() []byte {
	var b bytes.Buffer
	b.WriteString("import \"context\"\n\ntype Handler interface {\n")
	for _, t := range m.Tools {
		fmt.Fprintf(&b, "%s(context.Context, %s) (%s, error)\n", t.Method, t.Input, t.Output)
	}
	b.WriteString("}\n\ntype UnimplementedHandler struct{}\n")
	for _, t := range m.Tools {
		fmt.Fprintf(&b, "func (UnimplementedHandler) %s(context.Context, %s) (%s, error) { var zero %s; return zero, ErrNotImplemented }\n", t.Method, t.Input, t.Output, t.Output)
	}
	return b.Bytes()
}
