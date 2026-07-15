package bootstrap

import (
	"strings"
	"testing"
)

func TestBuildRepositoriesRequiresPersistenceWhenConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		environment map[string]string
		wantError   string
	}{
		{
			name:        "required with no endpoints",
			environment: map[string]string{"AGENTCARD_PERSISTENCE_REQUIRED": "true"},
			wantError:   "AGENTCARD_DATABASE_URL and AGENTCARD_S3_ENDPOINT are required",
		},
		{
			name: "required with database only",
			environment: map[string]string{
				"AGENTCARD_PERSISTENCE_REQUIRED": "true",
				"AGENTCARD_DATABASE_URL":         "postgres://private-value",
			},
			wantError: "AGENTCARD_DATABASE_URL and AGENTCARD_S3_ENDPOINT must be configured together",
		},
		{
			name:        "invalid flag",
			environment: map[string]string{"AGENTCARD_PERSISTENCE_REQUIRED": "tru"},
			wantError:   "AGENTCARD_PERSISTENCE_REQUIRED must be true or false",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := buildRepositories(testCase.environment)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("buildRepositories() error = %v, want %q", err, testCase.wantError)
			}
			for _, value := range testCase.environment {
				if value != "true" && value != "tru" && strings.Contains(err.Error(), value) {
					t.Fatalf("error exposed configuration value %q: %v", value, err)
				}
			}
		})
	}
}

func TestBuildRepositoriesAllowsExplicitMemoryDevelopmentMode(t *testing.T) {
	t.Parallel()

	repositories, err := buildRepositories(map[string]string{
		"AGENTCARD_PERSISTENCE_REQUIRED": "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repositories.database != nil {
		t.Fatal("memory development mode opened a database")
	}
	if _, ok := repositories.readiness.(readyRuntime); !ok {
		t.Fatalf("readiness = %T, want readyRuntime", repositories.readiness)
	}
}
