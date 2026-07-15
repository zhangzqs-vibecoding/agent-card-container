package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
)

func TestDeepSeekLiveCodeCardQualityGate(t *testing.T) {
	if os.Getenv("AGENTCARD_DEEPSEEK_LIVE") != "1" || os.Getenv("AGENTCARD_CODECARD_LIVE") != "1" {
		t.Skip("paid CodeCard live gate disabled; enable both explicit gates and run once")
	}
	image := os.Getenv("AGENTCARD_SANDBOX_TEST_IMAGE")
	if image == "" {
		t.Fatal("sandbox image is not configured")
	}
	pricing := codeCardEvalPricing{
		InputUSDPerMillion:  requiredPositiveFloat(t, "AGENTCARD_MODEL_INPUT_USD_PER_MILLION"),
		OutputUSDPerMillion: requiredPositiveFloat(t, "AGENTCARD_MODEL_OUTPUT_USD_PER_MILLION"),
	}
	provider, err := modelprovider.NewHTTPProviderFromEnvironment(map[string]string{
		"AGENTCARD_MODEL_BASE_URL": os.Getenv("AGENTCARD_MODEL_BASE_URL"),
		"AGENTCARD_MODEL_API_KEY":  os.Getenv("AGENTCARD_MODEL_API_KEY"),
		"AGENTCARD_MODEL":          os.Getenv("AGENTCARD_MODEL"),
	})
	if err != nil {
		t.Fatal("model environment configuration is invalid")
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal("repository path is invalid")
	}
	builder := NewTemplateWebBuilder(
		filepath.Join(repository, "tooling", "codecard-template"),
		sandbox.NewDockerBuilder(sandbox.DockerConfig{
			Image:   image,
			Timeout: 5 * time.Minute,
		}, sandbox.ExecRunner{}),
	)
	cases, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil || validateCodeCardEvalCases(cases) != nil {
		t.Fatal("CodeCard evaluation fixture is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	report := runCodeCardEvalSuite(
		ctx,
		NewCodingAgent(provider, NewNativeValidator(), WithWebBuilder(builder)),
		cases,
		pricing,
		t.Logf,
	)
	if failures := report.Failures(); len(failures) != 0 {
		t.Fatalf(
			"CodeCard quality gate failed: success=%d total=%d failures=%v",
			report.Successes,
			report.Total,
			failures,
		)
	}
}

func requiredPositiveFloat(t *testing.T, name string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s must be a positive number", name)
	}
	return value
}
