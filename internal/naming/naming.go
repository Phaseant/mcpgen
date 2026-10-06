package naming

import (
	"strings"
	"unicode"
)

// Go converts a spec name to an exported Go identifier.
func Go(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var b strings.Builder
	for _, word := range words {
		upper := strings.ToUpper(word)
		switch upper {
		case "IDS":
			b.WriteString("IDs")
		case "URLS":
			b.WriteString("URLs")
		case "URIS":
			b.WriteString("URIs")
		case "UUIDS":
			b.WriteString("UUIDs")
		case "ID", "API", "HTTP", "HTTPS", "URL", "URI", "UUID", "JSON", "MCP", "SDK", "IP":
			b.WriteString(upper)
		default:
			r := []rune(word)
			r[0] = unicode.ToUpper(r[0])
			b.WriteString(string(r))
		}
	}
	return b.String()
}
