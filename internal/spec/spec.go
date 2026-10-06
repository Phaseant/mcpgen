package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"go.yaml.in/yaml/v3"
)

var (
	toolName     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	schemaName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
	propertyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

type Document struct {
	MCP     string             `yaml:"mcp"`
	Info    Info               `yaml:"info"`
	Tools   map[string]*Tool   `yaml:"tools"`
	Schemas map[string]*Schema `yaml:"schemas"`
}

type Info struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type Tool struct {
	Title       string       `yaml:"title"`
	Description string       `yaml:"description"`
	Input       *Schema      `yaml:"input"`
	Output      *Schema      `yaml:"output"`
	Annotations *Annotations `yaml:"annotations"`
	Position    *yaml.Node   `yaml:"-"`
}

type Annotations struct {
	ReadOnlyHint    bool   `yaml:"readOnlyHint" json:"readOnlyHint"`
	DestructiveHint *bool  `yaml:"destructiveHint" json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `yaml:"idempotentHint" json:"idempotentHint"`
	OpenWorldHint   *bool  `yaml:"openWorldHint" json:"openWorldHint,omitempty"`
	Title           string `yaml:"title" json:"title,omitempty"`
}

type Schema struct {
	Ref                  string             `yaml:"$ref" json:"$ref,omitempty"`
	Type                 string             `yaml:"type" json:"type,omitempty"`
	Description          string             `yaml:"description" json:"description,omitempty"`
	Required             []string           `yaml:"required" json:"required,omitempty"`
	Properties           map[string]*Schema `yaml:"properties" json:"properties,omitempty"`
	Items                *Schema            `yaml:"items" json:"items,omitempty"`
	Enum                 []any              `yaml:"enum" json:"enum,omitempty"`
	AdditionalProperties *bool              `yaml:"additionalProperties" json:"additionalProperties,omitempty"`
	Minimum              *float64           `yaml:"minimum" json:"minimum,omitempty"`
	Maximum              *float64           `yaml:"maximum" json:"maximum,omitempty"`
	ExclusiveMinimum     *float64           `yaml:"exclusiveMinimum" json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64           `yaml:"exclusiveMaximum" json:"exclusiveMaximum,omitempty"`
	MultipleOf           *float64           `yaml:"multipleOf" json:"multipleOf,omitempty"`
	MinLength            *int64             `yaml:"minLength" json:"minLength,omitempty"`
	MaxLength            *int64             `yaml:"maxLength" json:"maxLength,omitempty"`
	Pattern              string             `yaml:"pattern" json:"pattern,omitempty"`
	MinItems             *int64             `yaml:"minItems" json:"minItems,omitempty"`
	MaxItems             *int64             `yaml:"maxItems" json:"maxItems,omitempty"`
	UniqueItems          bool               `yaml:"uniqueItems" json:"uniqueItems,omitempty"`
	MinProperties        *int64             `yaml:"minProperties" json:"minProperties,omitempty"`
	MaxProperties        *int64             `yaml:"maxProperties" json:"maxProperties,omitempty"`
	Position             *yaml.Node         `yaml:"-" json:"-"`
}

// checkedDecode keeps source positions and rejects unsupported keywords.
func checkedDecode(n *yaml.Node, dst any) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d:%d: expected a mapping", n.Line, n.Column)
	}
	fields := map[string]bool{}
	typ := reflect.TypeOf(dst).Elem()
	for i := 0; i < typ.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("yaml"), ",")[0]
		if key != "-" {
			fields[key] = true
		}
	}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i]
		if !fields[key.Value] {
			return fmt.Errorf("line %d:%d: unsupported field %q", key.Line, key.Column, key.Value)
		}
		if n.Content[i+1].Tag == "!!null" {
			return fmt.Errorf("line %d:%d: %s must not be null", key.Line, key.Column, key.Value)
		}
	}
	return n.Decode(dst)
}

func (s *Schema) UnmarshalYAML(n *yaml.Node) error {
	type plain Schema
	if err := checkedDecode(n, (*plain)(s)); err != nil {
		return err
	}
	s.Position = n
	return nil
}

func (t *Tool) UnmarshalYAML(n *yaml.Node) error {
	type plain Tool
	if err := checkedDecode(n, (*plain)(t)); err != nil {
		return err
	}
	t.Position = n
	return nil
}

func (a *Annotations) UnmarshalYAML(n *yaml.Node) error {
	type plain Annotations
	return checkedDecode(n, (*plain)(a))
}

func At(n *yaml.Node, path, message string) error {
	if n != nil {
		return fmt.Errorf("%d:%d: %s: %s", n.Line, n.Column, path, message)
	}
	return fmt.Errorf("%s: %s", path, message)
}

func Parse(filename string, data []byte) (*Document, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%s: expected one YAML document", filename)
	}
	if doc.MCP != "1" || doc.Info.Name == "" || doc.Info.Version == "" {
		return nil, fmt.Errorf("%s: mcp must be 1 and info.name and info.version are required", filename)
	}
	if len(doc.Tools) == 0 {
		return nil, fmt.Errorf("%s: at least one tool is required", filename)
	}
	for _, name := range Keys(doc.Schemas) {
		if !schemaName.MatchString(name) {
			return nil, fmt.Errorf("%s: invalid schema name %q", filename, name)
		}
		if err := doc.validate(doc.Schemas[name], "schemas."+name, map[string]bool{name: true}); err != nil {
			return nil, fmt.Errorf("%s:%w", filename, err)
		}
	}
	for _, name := range Keys(doc.Tools) {
		t := doc.Tools[name]
		if t == nil {
			return nil, fmt.Errorf("%s: tools.%s: missing definition", filename, name)
		}
		if !toolName.MatchString(name) {
			return nil, fmt.Errorf("%s:%w", filename, At(t.Position, "tools."+name, "tool names must contain 1-128 ASCII letters, digits, underscores, dots, or hyphens"))
		}
		for _, entry := range []struct {
			name   string
			schema *Schema
		}{{"input", t.Input}, {"output", t.Output}} {
			path := "tools." + name + "." + entry.name
			if entry.schema == nil {
				return nil, fmt.Errorf("%s:%w", filename, At(t.Position, path, "missing schema"))
			}
			if err := doc.validate(entry.schema, path, map[string]bool{}); err != nil {
				return nil, fmt.Errorf("%s:%w", filename, err)
			}
			if entry.name == "input" && doc.Deref(entry.schema).Type != "object" {
				return nil, fmt.Errorf("%s:%w", filename, At(entry.schema.Position, path, "tool input must be an object"))
			}
			if _, err := doc.JSONSchema(entry.schema); err != nil {
				return nil, fmt.Errorf("%s:%w", filename, At(entry.schema.Position, path, err.Error()))
			}
		}
	}
	return &doc, nil
}

func (d *Document) validate(s *Schema, path string, visiting map[string]bool) error {
	if s == nil {
		return fmt.Errorf("%s: missing schema", path)
	}
	bad := func(message string) error { return At(s.Position, path, message) }
	if s.Ref != "" {
		name, ok := strings.CutPrefix(s.Ref, "#/schemas/")
		if !ok || name == "" || d.Schemas[name] == nil {
			return bad("unknown local reference " + s.Ref)
		}
		if s.Position != nil {
			for i := 0; i < len(s.Position.Content); i += 2 {
				key := s.Position.Content[i].Value
				if key != "$ref" && key != "description" {
					return bad("$ref supports only a description sibling")
				}
			}
		}
		if visiting[name] {
			return bad("recursive references are not supported: " + s.Ref)
		}
		visiting[name] = true
		err := d.validate(d.Schemas[name], "schemas."+name, visiting)
		delete(visiting, name)
		return err
	}
	if s.Type != "object" && (s.Properties != nil || s.Required != nil || s.AdditionalProperties != nil || s.MinProperties != nil || s.MaxProperties != nil) {
		return bad("object keywords require type object")
	}
	if s.Type != "array" && (s.Items != nil || s.MinItems != nil || s.MaxItems != nil || s.UniqueItems) {
		return bad("array keywords require type array")
	}
	if s.Type != "string" && (s.Pattern != "" || s.MinLength != nil || s.MaxLength != nil) {
		return bad("string keywords require type string")
	}
	if s.Type != "number" && s.Type != "integer" && (s.Minimum != nil || s.Maximum != nil || s.ExclusiveMinimum != nil || s.ExclusiveMaximum != nil || s.MultipleOf != nil) {
		return bad("numeric keywords require type number or integer")
	}
	for _, pair := range [][2]*int64{{s.MinLength, s.MaxLength}, {s.MinItems, s.MaxItems}, {s.MinProperties, s.MaxProperties}} {
		for _, bound := range pair {
			if bound != nil && *bound < 0 {
				return bad("length and count constraints must be non-negative")
			}
		}
		if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
			return bad("minimum constraint exceeds maximum")
		}
	}
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		return bad("minimum exceeds maximum")
	}
	if s.MultipleOf != nil && *s.MultipleOf <= 0 {
		return bad("multipleOf must be positive")
	}
	if s.Pattern != "" {
		if _, err := regexp.Compile(s.Pattern); err != nil {
			return bad("invalid pattern: " + err.Error())
		}
	}
	switch s.Type {
	case "string", "integer", "number", "boolean":
	case "array":
		if err := d.validate(s.Items, path+".items", visiting); err != nil {
			return err
		}
	case "object":
		seen := map[string]bool{}
		for _, name := range s.Required {
			if seen[name] || s.Properties[name] == nil {
				return bad(fmt.Sprintf("required: duplicate or unknown property %q", name))
			}
			seen[name] = true
		}
		for _, name := range Keys(s.Properties) {
			if !propertyName.MatchString(name) {
				return bad(fmt.Sprintf("unsupported property name %q", name))
			}
			if err := d.validate(s.Properties[name], path+".properties."+name, visiting); err != nil {
				return err
			}
		}
	default:
		return bad(fmt.Sprintf("unsupported type %q", s.Type))
	}
	if s.Enum != nil {
		if s.Type == "object" || s.Type == "array" || len(s.Enum) == 0 {
			return bad("enum requires at least one primitive value")
		}
		seen := map[string]bool{}
		for _, value := range s.Enum {
			data, err := json.Marshal(value)
			if err != nil {
				return bad("invalid enum value")
			}
			if seen[string(data)] {
				return bad("duplicate enum value")
			}
			seen[string(data)] = true
			switch s.Type {
			case "string":
				if _, ok := value.(string); !ok {
					return bad("string enum requires strings")
				}
			case "boolean":
				if _, ok := value.(bool); !ok {
					return bad("boolean enum requires booleans")
				}
			case "integer":
				if _, ok := value.(int); !ok {
					return bad("integer enum requires integers")
				}
			case "number":
				switch value.(type) {
				case int, float64:
				default:
					return bad("number enum requires numbers")
				}
			}
		}
	}
	return nil
}

func (d *Document) Deref(s *Schema) *Schema {
	for s.Ref != "" {
		s = d.Schemas[strings.TrimPrefix(s.Ref, "#/schemas/")]
	}
	return s
}

// JSONSchema normalizes local references to self-contained JSON Schema $defs.
func (d *Document) JSONSchema(s *Schema) ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	defsData, err := json.Marshal(d.Schemas)
	if err != nil {
		return nil, err
	}
	var defs map[string]any
	if err := json.Unmarshal(defsData, &defs); err != nil {
		return nil, err
	}
	var rewrite func(map[string]any)
	rewrite = func(value map[string]any) {
		if ref, ok := value["$ref"].(string); ok {
			value["$ref"] = "#/$defs/" + strings.TrimPrefix(ref, "#/schemas/")
		}
		if props, ok := value["properties"].(map[string]any); ok {
			for _, prop := range props {
				rewrite(prop.(map[string]any))
			}
		}
		if items, ok := value["items"].(map[string]any); ok {
			rewrite(items)
		}
	}
	rewrite(root)
	for _, value := range defs {
		rewrite(value.(map[string]any))
	}
	// The SDK requires an explicit object type on the input schema root.
	if s.Ref != "" {
		root["type"] = d.Deref(s).Type
	}
	if len(defs) > 0 {
		root["$defs"] = defs
	}
	data, err = json.Marshal(root)
	if err != nil {
		return nil, err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	if _, err := schema.Resolve(nil); err != nil {
		return nil, err
	}
	return data, nil
}

func Keys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func Required(s *Schema, name string) bool {
	for _, v := range s.Required {
		if v == name {
			return true
		}
	}
	return false
}

func Comment(text string) string { return "// " + strings.ReplaceAll(text, "\n", "\n// ") + "\n" }
