package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

const codeCardEvalFixturePath = "testdata/codecard_eval_cases.v1.json"

func TestCodeCardEvalFixtureIsFrozen(t *testing.T) {
	t.Parallel()

	cases, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCodeCardEvalCases(cases); err != nil {
		t.Fatal(err)
	}
}

func TestCodeCardEvalDecoderRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	t.Parallel()

	valid := `[{"id":"canvas-draw","category":"canvas","prompt":"画板","requiredSemantics":["canvas","pointer"],"forbiddenPatterns":["eval("],"allowedCapabilities":[],"interactions":["draw"]}]`
	for _, source := range []string{
		strings.Replace(valid, `"interactions"`, `"unknown":true,"interactions"`, 1),
		valid + `{}`,
	} {
		if _, err := decodeCodeCardEvalCases(strings.NewReader(source)); err == nil {
			t.Fatalf("decodeCodeCardEvalCases(%q) error = nil", source)
		}
	}
}

func TestCodeCardEvalValidationRejectsContractDrift(t *testing.T) {
	t.Parallel()

	valid, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func([]codeCardEvalCase) []codeCardEvalCase
	}{
		{name: "not twenty", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase { return cases[:19] }},
		{name: "duplicate ID", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[1].ID = cases[0].ID
			return cases
		}},
		{name: "unknown category", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].Category = "unknown"
			return cases
		}},
		{name: "category drift", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].Category = cases[4].Category
			return cases
		}},
		{name: "blank prompt", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].Prompt = " \n"
			return cases
		}},
		{name: "oversized prompt", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].Prompt = strings.Repeat("长", 501)
			return cases
		}},
		{name: "empty semantics", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].RequiredSemantics = nil
			return cases
		}},
		{name: "empty interactions", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].Interactions = nil
			return cases
		}},
		{name: "unknown capability", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].AllowedCapabilities = []string{"process.exec"}
			return cases
		}},
		{name: "duplicate assertion", mutate: func(cases []codeCardEvalCase) []codeCardEvalCase {
			cases[0].RequiredSemantics = []string{"canvas", "canvas"}
			return cases
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cases := cloneCodeCardEvalCases(valid)
			if err := validateCodeCardEvalCases(test.mutate(cases)); err == nil {
				t.Fatal("validateCodeCardEvalCases() error = nil")
			}
		})
	}
}

func TestCodeCardEvalDecoderRequiresAllFields(t *testing.T) {
	t.Parallel()

	cases, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(cases[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"id", "category", "prompt", "requiredSemantics", "forbiddenPatterns", "allowedCapabilities", "interactions",
	} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &object); err != nil {
			t.Fatal(err)
		}
		delete(object, field)
		missing, err := json.Marshal([]map[string]json.RawMessage{object})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeCodeCardEvalCases(bytes.NewReader(missing)); err == nil {
			t.Fatalf("decoder accepted missing %s", field)
		}
	}
}

func TestCodeCardEvalReportEnforcesQualityAndBudgets(t *testing.T) {
	t.Parallel()

	passing := codeCardEvalReport{
		Total:     20,
		Successes: 16,
		CategorySuccesses: map[string]int{
			"canvas": 3, "game": 3, "visualization": 3, "interaction": 3, "offline-tool": 4,
		},
		ModelCalls:       60,
		InputTokens:      300_000,
		OutputTokens:     160_000,
		EstimatedCostUSD: 5,
		CostKnown:        true,
		Duration:         90 * time.Minute,
	}
	if failures := passing.Failures(); len(failures) != 0 {
		t.Fatalf("passing report failures = %v", failures)
	}

	tests := []struct {
		name   string
		mutate func(*codeCardEvalReport)
	}{
		{name: "low total", mutate: func(report *codeCardEvalReport) { report.Successes = 15 }},
		{name: "low category", mutate: func(report *codeCardEvalReport) { report.CategorySuccesses["canvas"] = 2 }},
		{name: "too many calls", mutate: func(report *codeCardEvalReport) { report.ModelCalls = 61 }},
		{name: "too many input tokens", mutate: func(report *codeCardEvalReport) { report.InputTokens++ }},
		{name: "too many output tokens", mutate: func(report *codeCardEvalReport) { report.OutputTokens++ }},
		{name: "unknown cost", mutate: func(report *codeCardEvalReport) { report.CostKnown = false }},
		{name: "too expensive", mutate: func(report *codeCardEvalReport) { report.EstimatedCostUSD = 5.01 }},
		{name: "too slow", mutate: func(report *codeCardEvalReport) { report.Duration += time.Nanosecond }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := passing.clone()
			test.mutate(&report)
			if failures := report.Failures(); len(failures) == 0 {
				t.Fatal("Failures() = nil")
			}
		})
	}
}

func TestCodeCardEvalRunnerIsSerialBoundedAndDoesNotLeakContent(t *testing.T) {
	t.Parallel()

	cases, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	generator := &fakeCodeCardEvalGenerator{failSession: "codecard-eval-game-tic-tac-toe"}
	var lines []string
	report := runCodeCardEvalSuite(
		context.Background(),
		generator,
		cases,
		codeCardEvalPricing{InputUSDPerMillion: 0.1, OutputUSDPerMillion: 0.2},
		func(format string, arguments ...any) { lines = append(lines, fmt.Sprintf(format, arguments...)) },
	)

	if report.Total != 20 || report.Successes != 19 || report.ModelCalls != 20 || report.InputTokens != 2000 || report.OutputTokens != 1000 {
		t.Fatalf("report = %#v", report)
	}
	if generator.maxActive != 1 || len(generator.sessions) != 20 || report.FailureKinds["provider"] != 1 {
		t.Fatalf("generator=%#v failures=%#v", generator, report.FailureKinds)
	}
	if !report.CostKnown || report.EstimatedCostUSD <= 0 {
		t.Fatalf("cost = known:%t value:%f", report.CostKnown, report.EstimatedCostUSD)
	}
	output := strings.Join(lines, "\n")
	for _, forbidden := range []string{
		cases[0].Prompt,
		"sensitive generated source",
		"provider-sensitive-marker",
		"AGENTCARD_MODEL_API_KEY",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("runner output leaked %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "id=canvas-free-draw") || !strings.Contains(output, "summary success=19 total=20") {
		t.Fatalf("runner output is incomplete: %s", output)
	}
}

func TestCodeCardEvalRunnerRequiresKnownPositivePricing(t *testing.T) {
	t.Parallel()

	cases, err := loadCodeCardEvalCasesFile(codeCardEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	report := runCodeCardEvalSuite(
		context.Background(),
		&fakeCodeCardEvalGenerator{},
		cases,
		codeCardEvalPricing{},
		func(string, ...any) {},
	)
	if report.CostKnown || !containsString(report.Failures(), "cost:unknown") {
		t.Fatalf("report = %#v failures=%v", report, report.Failures())
	}
}

func TestCodeCardEvalFailureKindUsesStablePipelineCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "provider", err: errors.New("upstream response body"), want: "provider"},
		{name: "source envelope", err: fmt.Errorf("%w: %w", ErrValidationFailed, ErrWebSourceInvalid), want: "source-envelope"},
		{name: "sandbox", err: fmt.Errorf("%w: %w", ErrValidationFailed, ErrWebBuildFailed), want: "sandbox-build"},
		{name: "artifact", err: fmt.Errorf("%w: %w", ErrValidationFailed, ErrWebArtifactInvalid), want: "artifact"},
		{name: "validation", err: ErrValidationFailed, want: "validation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := codeCardEvalFailureKind(test.err); got != test.want {
				t.Fatalf("codeCardEvalFailureKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDeepSeekLiveWorkflowContainsExplicitBoundedCodeCardGate(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "..", ".github", "workflows", "deepseek-live.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, required := range []string{
		"confirm_codecard_paid_test",
		"timeout-minutes: 100",
		"TestDeepSeekLiveCodeCardQualityGate",
		"AGENTCARD_CODECARD_LIVE: '1'",
		"AGENTCARD_SANDBOX_TEST_IMAGE",
		"AGENTCARD_MODEL_INPUT_USD_PER_MILLION",
		"AGENTCARD_MODEL_OUTPUT_USD_PER_MILLION",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("workflow is missing %q", required)
		}
	}
}

type fakeCodeCardEvalGenerator struct {
	active      int
	maxActive   int
	sessions    []string
	failSession string
}

func (generator *fakeCodeCardEvalGenerator) Generate(_ context.Context, request Request) (Result, error) {
	generator.active++
	defer func() { generator.active-- }()
	if generator.active > generator.maxActive {
		generator.maxActive = generator.active
	}
	generator.sessions = append(generator.sessions, request.SessionID)
	result := Result{
		Runtime:      RuntimeWeb,
		Attempts:     1,
		InputTokens:  100,
		OutputTokens: 50,
		Files: map[string][]byte{
			"index.html": []byte("sensitive generated source"),
		},
	}
	if request.Requirement.Target != generation.TargetWeb {
		return result, errors.New("target drift")
	}
	if request.SessionID == generator.failSession {
		return result, errors.New("provider-sensitive-marker")
	}
	return result, nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
