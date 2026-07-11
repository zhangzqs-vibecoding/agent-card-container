package app_test

import (
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/app"
)

func TestNewPreservesServiceName(t *testing.T) {
	t.Parallel()

	got := app.New("test").Name()

	if got != "test" {
		t.Fatalf("Name() = %q, want %q", got, "test")
	}
}
