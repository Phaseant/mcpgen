package generate

import (
	"bytes"
	"fmt"
)

func (m *model) tools() []byte {
	var b bytes.Buffer
	b.WriteString("import \"github.com/modelcontextprotocol/go-sdk/mcp\"\n")
	for _, t := range m.Tools {
		annotations := "nil"
		if t.Spec.Annotations != nil {
			a := t.Spec.Annotations
			annotations = fmt.Sprintf("&mcp.ToolAnnotations{ReadOnlyHint: %t, IdempotentHint: %t, Title: %q", a.ReadOnlyHint, a.IdempotentHint, a.Title)
			for _, hint := range []struct {
				name  string
				value *bool
			}{{"DestructiveHint", a.DestructiveHint}, {"OpenWorldHint", a.OpenWorldHint}} {
				if hint.value != nil {
					varName := "hint" + t.Method + hint.name
					fmt.Fprintf(&b, "var %s = %t\n", varName, *hint.value)
					annotations += fmt.Sprintf(", %s: &%s", hint.name, varName)
				}
			}
			annotations += "}"
		}
		fmt.Fprintf(&b, "var tool%s = &mcp.Tool{Name: %q, Title: %q, Description: %q, Annotations: %s, InputSchema: schema%sInput, OutputSchema: schema%sOutput}\n", t.Method, t.Name, t.Spec.Title, t.Spec.Description, annotations, t.Method, t.Method)
	}
	return b.Bytes()
}
