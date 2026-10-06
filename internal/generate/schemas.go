package generate

import (
	"bytes"
	"fmt"
)

func (m *model) schemas() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("import \"encoding/json\"\n")
	for _, t := range m.Tools {
		in, err := m.Doc.JSONSchema(t.Spec.Input)
		if err != nil {
			return nil, fmt.Errorf("tool %s input schema: %w", t.Name, err)
		}
		out, err := m.Doc.JSONSchema(t.Spec.Output)
		if err != nil {
			return nil, fmt.Errorf("tool %s output schema: %w", t.Name, err)
		}
		fmt.Fprintf(&b, "var schema%sInput = json.RawMessage(%q)\nvar schema%sOutput = json.RawMessage(%q)\n", t.Method, in, t.Method, out)
	}
	return b.Bytes(), nil
}
