package spec

import (
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"strings"
	"testing"
)

const envelope = `mcp: "1"
info: {name: test, version: 1.0.0}
tools:
  echo:
    input: %s
    output: {type: object}
%s
`

func TestParseDiagnostics(t *testing.T) {
	cases := []struct{ name, input, schemas, want string }{
		{"missing required property", `{type: object, required: [missing]}`, "", "duplicate or unknown property"},
		{"unknown keyword", `{type: object, typo: true}`, "", `unsupported field "typo"`},
		{"unknown reference", `{$ref: '#/schemas/Missing'}`, "", "unknown local reference"},
		{"reference siblings", `{$ref: '#/schemas/Input', type: object}`, "schemas: {Input: {type: object}}", "$ref supports only"},
		{"empty reference siblings", `{$ref: '#/schemas/Input', required: []}`, "schemas: {Input: {type: object}}", "$ref supports only"},
		{"null keyword", `{type: object, additionalProperties: null}`, "", "must not be null"},
		{"recursive reference", `{$ref: '#/schemas/Input'}`, "schemas: {Input: {type: object, properties: {self: {$ref: '#/schemas/Input'}}}}", "recursive references"},
		{"nonobject input", `{type: string}`, "", "tool input must be an object"},
		{"negative count", `{type: object, properties: {x: {type: string, minLength: -1}}}`, "", "must be non-negative"},
		{"inverted range", `{type: object, properties: {x: {type: integer, minimum: 3, maximum: 2}}}`, "", "minimum exceeds maximum"},
		{"invalid pattern", `{type: object, properties: {x: {type: string, pattern: '['}}}`, "", "invalid pattern"},
		{"invalid enum", `{type: object, properties: {x: {type: string, enum: [1]}}}`, "", "string enum requires strings"},
		{"duplicate enum", `{type: object, properties: {x: {type: string, enum: [a, a]}}}`, "", "duplicate enum"},
		{"empty enum", `{type: object, properties: {x: {type: string, enum: []}}}`, "", "at least one primitive"},
		{"missing items", `{type: object, properties: {x: {type: array}}}`, "", "missing schema"},
		{"wrong constraint type", `{type: object, minimum: 1}`, "", "numeric keywords require"},
		{"invalid multiple", `{type: object, properties: {x: {type: number, multipleOf: 0}}}`, "", "multipleOf must be positive"},
		{"invalid property tag", `{type: object, properties: {'a,b': {type: string}}}`, "", "unsupported property name"},
		{"duplicate YAML key", `{type: object, type: string}`, "", "already defined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := strings.Replace(envelope, "%s", tc.input, 1)
			data = strings.Replace(data, "%s", tc.schemas, 1)
			_, err := Parse("test.yaml", []byte(data))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "test.yaml:") {
				t.Fatalf("error=%v; want %q with filename", err, tc.want)
			}
		})
	}
}

func TestSourcePosition(t *testing.T) {
	_, err := Parse("test.yaml", []byte("mcp: '1'\ninfo: {name: test, version: '1'}\ntools:\n  echo:\n    input:\n      type: invalid\n    output: {type: object}\n"))
	if err == nil || !strings.Contains(err.Error(), "test.yaml:6:7: tools.echo.input") {
		t.Fatalf("error=%v", err)
	}
}

func TestMultipleDocuments(t *testing.T) {
	_, err := Parse("test.yaml", []byte(strings.Replace(strings.Replace(envelope, "%s", "{type: object}", 1), "%s", "---\n{}", 1)))
	if err == nil || !strings.Contains(err.Error(), "one YAML document") {
		t.Fatalf("error=%v", err)
	}
}

func TestSchemaConstraints(t *testing.T) {
	cases := []struct {
		name, schema   string
		valid, invalid any
	}{
		{"string bounds", `{type: string, minLength: 1, maxLength: 2}`, "ok", "long"},
		{"string pattern", `{type: string, pattern: '^a+$'}`, "aa", "bb"},
		{"number bounds", `{type: number, minimum: 1, maximum: 3}`, 2, 4},
		{"exclusive bounds", `{type: number, exclusiveMinimum: 1, exclusiveMaximum: 3}`, 2, 1},
		{"multiple", `{type: number, multipleOf: 0.5}`, 1.5, 1.25},
		{"array bounds", `{type: array, minItems: 1, maxItems: 2, items: {type: integer}}`, []int{1}, []int{}},
		{"array maximum", `{type: array, maxItems: 1, items: {type: integer}}`, []int{1}, []int{1, 2}},
		{"array unique", `{type: array, uniqueItems: true, items: {type: integer}}`, []int{1, 2}, []int{1, 1}},
		{"object bounds", `{type: object, minProperties: 1, maxProperties: 1, properties: {x: {type: string}, y: {type: string}}}`, map[string]any{"x": "a"}, map[string]any{"x": "a", "y": "b"}},
		{"boolean enum", `{type: boolean, enum: [true]}`, true, false},
		{"integer enum", `{type: integer, enum: [1, 2]}`, 1, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "{type: object, properties: {value: " + tc.schema + "}}"
			data := strings.Replace(strings.Replace(envelope, "%s", input, 1), "%s", "", 1)
			doc, err := Parse("constraints.yaml", []byte(data))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := doc.JSONSchema(doc.Tools["echo"].Input)
			if err != nil {
				t.Fatal(err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			validate := func(value any) error {
				data, err := json.Marshal(map[string]any{"value": value})
				if err != nil {
					t.Fatal(err)
				}
				var instance any
				if err := json.Unmarshal(data, &instance); err != nil {
					t.Fatal(err)
				}
				return resolved.Validate(instance)
			}
			if err := validate(tc.valid); err != nil {
				t.Fatalf("valid value rejected: %v", err)
			}
			if err := validate(tc.invalid); err == nil {
				t.Fatal("invalid value accepted")
			}
		})
	}
}
