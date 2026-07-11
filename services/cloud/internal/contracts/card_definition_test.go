package contracts_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/contracts"
)

func TestDecodeCardDefinitionFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fixture    string
		runtime    contracts.CardRuntime
		entrypoint string
		capability string
	}{
		{
			name:       "native",
			fixture:    "native-card.json",
			runtime:    contracts.CardRuntimeNative,
			entrypoint: "payload/native.json",
			capability: "notification.show",
		},
		{
			name:       "web",
			fixture:    "web-card.json",
			runtime:    contracts.CardRuntimeWeb,
			entrypoint: "payload/web/index.html",
			capability: "storage",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			definition, err := contracts.DecodeCardDefinition(
				strings.NewReader(string(readFixture(t, tt.fixture))),
			)
			if err != nil {
				t.Fatalf("DecodeCardDefinition() error = %v", err)
			}

			if definition.Runtime != tt.runtime {
				t.Fatalf("Runtime = %q, want %q", definition.Runtime, tt.runtime)
			}
			if definition.Entrypoint != tt.entrypoint {
				t.Fatalf("Entrypoint = %q, want %q", definition.Entrypoint, tt.entrypoint)
			}
			if definition.PreferredSize.Width <= definition.MinSize.Width {
				t.Fatalf("PreferredSize.Width = %v, want greater than %v", definition.PreferredSize.Width, definition.MinSize.Width)
			}
			if !definition.HasCapability(tt.capability) {
				t.Fatalf("HasCapability(%q) = false", tt.capability)
			}
		})
	}
}

func TestDecodeCardDefinitionRejectsUnknownRuntime(t *testing.T) {
	t.Parallel()

	input := strings.Replace(
		string(readFixture(t, "web-card.json")),
		"\"runtime\": \"web\"",
		"\"runtime\": \"script\"",
		1,
	)

	_, err := contracts.DecodeCardDefinition(strings.NewReader(input))

	if !errors.Is(err, contracts.ErrInvalidRuntime) {
		t.Fatalf("error = %v, want ErrInvalidRuntime", err)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	path := filepath.Join(
		filepath.Dir(currentFile),
		"..", "..", "..", "..",
		"contracts", "card", "fixtures", name,
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return data
}
