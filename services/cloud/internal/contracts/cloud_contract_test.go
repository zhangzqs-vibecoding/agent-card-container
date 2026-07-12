package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

func TestCloudEventFixtureMatchesGoContract(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "contracts", "cloud", "fixtures", "generation-event.json"))
	if err != nil {
		t.Fatal(err)
	}
	var event generation.Event
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.EventID != 3 || event.SessionID != "gen_01" || event.Progress != 0.35 {
		t.Fatalf("event = %#v", event)
	}
}

func TestOpenAPIContainsEveryMVPRoute(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "contracts", "cloud", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, route := range []string{
		"/v1/generations:",
		"/v1/generations/{id}:",
		"/v1/generations/{id}/messages:",
		"/v1/generations/{id}/confirm:",
		"/v1/generations/{id}/cancel:",
		"/v1/generations/{id}/events:",
		"/v1/cards:",
		"/v1/cards/{cardId}:",
		"/v1/cards/{cardId}/versions/{versionId}/artifact:",
	} {
		if !strings.Contains(source, route) {
			t.Fatalf("OpenAPI missing %s", route)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
