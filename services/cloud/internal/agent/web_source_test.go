package agent

import (
	"strings"
	"testing"
)

func TestDecodeWebSourceRejectsInvalidText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "invalid UTF-8", source: string([]byte{0xff})},
		{name: "NUL", source: "export function Card() {\x00return null }"},
		{name: "oversized line", source: "export const value = '" + strings.Repeat("x", 32*1024) + "'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := `{"files":{"src/card.tsx":` + quotedJSON(test.source) + `}}`
			if _, err := decodeWebSource(content); err == nil {
				t.Fatal("decodeWebSource() error = nil")
			}
		})
	}
}

func quotedJSON(value string) string {
	result := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"\x00", `\u0000`,
	).Replace(value)
	return `"` + result + `"`
}
