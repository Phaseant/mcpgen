package generate

import (
	"fmt"
	"github.com/phaseant/mcpgen/internal/naming"
	"github.com/phaseant/mcpgen/internal/spec"
	"go/token"
	"strconv"
	"strings"
)

func (m *model) emitType(name string, s *spec.Schema) {
	if m.Err != nil {
		return
	}
	comment := ""
	if s.Description != "" {
		comment = spec.Comment(s.Description)
	}
	expr := ""
	if s.Ref != "" {
		expr = "= " + refName(s)
	} else {
		expr = m.typeExpr(name, s)
	}
	declaration := comment + fmt.Sprintf("type %s %s\n\n", name, expr)
	if len(s.Enum) > 0 {
		declaration += "const (\n"
		for i, value := range s.Enum {
			suffix := fmt.Sprintf("Value%d", i+1)
			literal := fmt.Sprint(value)
			if v, ok := value.(string); ok {
				literal = strconv.Quote(v)
				if named := naming.Go(v); named != "" {
					suffix = named
				}
			} else if v, ok := value.(bool); ok {
				if v {
					suffix = "True"
				} else {
					suffix = "False"
				}
			}
			constant := name + suffix
			m.claim(constant, name+".enum", s)
			declaration += fmt.Sprintf("%s %s = %s\n", constant, name, literal)
		}
		declaration += ")\n\n"
	}
	m.Declarations[name] = declaration
}

func (m *model) claim(name, origin string, s *spec.Schema) {
	if m.Err != nil {
		return
	}
	if !token.IsIdentifier(name) || token.Lookup(name).IsKeyword() || name == "_" {
		m.Err = spec.At(s.Position, origin, fmt.Sprintf("invalid Go identifier %q", name))
		return
	}
	if other, ok := m.Names[name]; ok {
		m.Err = spec.At(s.Position, origin, fmt.Sprintf("Go name %s collides with %s", name, other))
		return
	}
	m.Names[name] = origin
}

func refName(s *spec.Schema) string { return naming.Go(strings.TrimPrefix(s.Ref, "#/schemas/")) }

func (m *model) rootType(name string, s *spec.Schema) string {
	if s.Ref != "" {
		return refName(s)
	}
	m.claim(name, name, s)
	m.emitType(name, s)
	return name
}

func (m *model) childType(name string, s *spec.Schema) string {
	if s.Ref != "" {
		return refName(s)
	}
	if s.Type == "object" || len(s.Enum) > 0 {
		m.claim(name, name, s)
		m.emitType(name, s)
		return name
	}
	return m.typeExpr(name, s)
}

func (m *model) typeExpr(name string, s *spec.Schema) string {
	switch s.Type {
	case "string":
		return "string"
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		return "[]" + m.childType(name+"Item", s.Items)
	default:
		out := "struct {\n"
		fields := map[string]string{}
		for _, key := range spec.Keys(s.Properties) {
			field := naming.Go(key)
			prop := s.Properties[key]
			if !token.IsIdentifier(field) || field == "" {
				m.Err = spec.At(prop.Position, name+"."+key, "invalid Go field name")
				continue
			}
			if other, ok := fields[field]; ok {
				m.Err = spec.At(prop.Position, name+"."+key, "field name collides with "+other)
				continue
			}
			fields[field] = key
			expr := m.childType(name+field, prop)
			tag := key
			if !spec.Required(s, key) {
				expr = "*" + expr
				tag += ",omitempty"
			}
			if prop.Description != "" {
				out += spec.Comment(prop.Description)
			}
			out += fmt.Sprintf("%s %s `json:%q`\n", field, expr, tag)
		}
		return out + "}"
	}
}
