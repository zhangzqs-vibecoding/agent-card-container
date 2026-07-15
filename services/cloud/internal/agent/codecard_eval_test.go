package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
