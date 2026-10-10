package mongofs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
)

// jsonQuote quotes a string as JSON without the HTML escaping encoding/json
// applies by default (<, > and & would read as < in a document).
func jsonQuote(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
