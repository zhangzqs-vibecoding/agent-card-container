package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

const nativeEvalFixturePath = "testdata/native_eval_cases.v1.json"

var nativeEvalCategoryCounts = map[string]int{
	"timer":                     3,
	"todo":                      4,
	"dashboard":                 3,
	"form":                      3,
	"chart":                     3,
	"offline":                   2,
	"no-network-file-clipboard": 2,
}

type nativeEvalCase struct {
	ID                    string   `json:"id"`
	Category              string   `json:"category"`
	Prompt                string   `json:"prompt"`
	RequiredComponents    []string `json:"requiredComponents"`
	ForbiddenCapabilities []string `json:"forbiddenCapabilities"`
}

func TestNativeEvalFixtureHasExactlyTwentyStrictNativeCases(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCasesFile(nativeEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeEvalCases(cases); err != nil {
		t.Fatal(err)
	}
}

func TestNativeEvalFixtureDoesNotScoreUnrequestedLayoutOrStatusComponents(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCasesFile(nativeEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	layoutOnly := map[string]bool{
		"Container": true,
		"Row":       true,
		"Column":    true,
		"Stack":     true,
		"Grid":      true,
		"Scroll":    true,
		"Divider":   true,
	}
	for _, evalCase := range cases {
		for _, component := range evalCase.RequiredComponents {
			if layoutOnly[component] {
				t.Fatalf("case %s scores layout-only component %s", evalCase.ID, component)
			}
			if component == "Badge" && !strings.Contains(evalCase.Prompt, "徽标") &&
				!strings.Contains(evalCase.Prompt, "状态") {
				t.Fatalf("case %s requires an unrequested Badge", evalCase.ID)
			}
		}
	}
}

func TestNativeEvalFixtureDecoderRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	t.Parallel()

	for _, source := range []string{
		`[{"id":"case","category":"timer","prompt":"prompt","requiredComponents":["Text"],"forbiddenCapabilities":[],"unknown":true}]`,
		`[] {}`,
	} {
		if _, err := decodeNativeEvalCases(strings.NewReader(source)); err == nil {
			t.Fatalf("decodeNativeEvalCases(%q) error = nil", source)
		}
	}
}

func TestNativeEvalFixtureValidationRejectsContractDrift(t *testing.T) {
	t.Parallel()

	valid, err := loadNativeEvalCasesFile(nativeEvalFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func([]nativeEvalCase) []nativeEvalCase
	}{
		{name: "not exactly twenty", mutate: func(cases []nativeEvalCase) []nativeEvalCase { return cases[:19] }},
		{name: "duplicate ID", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[1].ID = cases[0].ID
			return cases
		}},
		{name: "invalid ID", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].ID = "Invalid ID"
			return cases
		}},
		{name: "blank prompt", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = " \t\n"
			return cases
		}},
		{name: "oversized prompt", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = strings.Repeat("长", 501)
			return cases
		}},
		{name: "unknown category", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Category = "unknown"
			return cases
		}},
		{name: "unknown component", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].RequiredComponents = []string{"Unknown"}
			return cases
		}},
		{name: "duplicate component", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].RequiredComponents = []string{"Text", "Text"}
			return cases
		}},
		{name: "no required component", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].RequiredComponents = nil
			return cases
		}},
		{name: "unknown forbidden capability", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].ForbiddenCapabilities = []string{"filesystem.read"}
			return cases
		}},
		{name: "duplicate forbidden capability", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].ForbiddenCapabilities = []string{"network.fetch", "network.fetch"}
			return cases
		}},
		{name: "not forced native compatible", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = "生成一个自由绘制画板"
			return cases
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cases := cloneNativeEvalCases(valid)
			if err := validateNativeEvalCases(test.mutate(cases)); err == nil {
				t.Fatal("validateNativeEvalCases() error = nil")
			}
		})
	}
}

func decodeNativeEvalCases(reader io.Reader) ([]nativeEvalCase, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var rawCases []json.RawMessage
	if err := decoder.Decode(&rawCases); err != nil {
		return nil, fmt.Errorf("decode native eval cases: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("native eval fixture must contain one JSON value")
	}

	requiredFields := []string{"id", "category", "prompt", "requiredComponents", "forbiddenCapabilities"}
	cases := make([]nativeEvalCase, 0, len(rawCases))
	for index, rawCase := range rawCases {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawCase, &fields); err != nil {
			return nil, fmt.Errorf("native eval case %d must be an object", index)
		}
		if len(fields) != len(requiredFields) {
			return nil, fmt.Errorf("native eval case %d must contain exactly five fields", index)
		}
		for _, field := range requiredFields {
			if _, exists := fields[field]; !exists {
				return nil, fmt.Errorf("native eval case %d is missing %s", index, field)
			}
		}
		var evalCase nativeEvalCase
		caseDecoder := json.NewDecoder(bytes.NewReader(rawCase))
		caseDecoder.DisallowUnknownFields()
		if err := caseDecoder.Decode(&evalCase); err != nil {
			return nil, fmt.Errorf("decode native eval case %d: %w", index, err)
		}
		cases = append(cases, evalCase)
	}
	return cases, nil
}

func loadNativeEvalCasesFile(path string) ([]nativeEvalCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open native eval fixture: %w", err)
	}
	defer file.Close()
	return decodeNativeEvalCases(file)
}

func validateNativeEvalCases(cases []nativeEvalCase) error {
	if len(cases) != 20 {
		return fmt.Errorf("native eval case count = %d, want 20", len(cases))
	}
	contractContext, err := loadNativeContractContext()
	if err != nil {
		return err
	}
	knownCapabilities := make(map[string]bool)
	for _, capability := range contractContext.Catalog.CapabilityMethods {
		if capability.ManifestCapability != nil {
			knownCapabilities[*capability.ManifestCapability] = true
		}
	}

	idPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	seenIDs := make(map[string]bool, len(cases))
	categoryCounts := make(map[string]int, len(nativeEvalCategoryCounts))
	selector := Selector{}
	for index, evalCase := range cases {
		if !idPattern.MatchString(evalCase.ID) || seenIDs[evalCase.ID] {
			return fmt.Errorf("native eval case %d has invalid or duplicate id", index)
		}
		seenIDs[evalCase.ID] = true
		if _, exists := nativeEvalCategoryCounts[evalCase.Category]; !exists {
			return fmt.Errorf("native eval case %s has unknown category", evalCase.ID)
		}
		categoryCounts[evalCase.Category]++
		if strings.TrimSpace(evalCase.Prompt) == "" || utf8.RuneCountInString(evalCase.Prompt) > 500 {
			return fmt.Errorf("native eval case %s has invalid prompt", evalCase.ID)
		}
		if len(evalCase.RequiredComponents) == 0 {
			return fmt.Errorf("native eval case %s has no required components", evalCase.ID)
		}
		seenComponents := make(map[string]bool, len(evalCase.RequiredComponents))
		for _, component := range evalCase.RequiredComponents {
			if _, known := contractContext.Catalog.Components[component]; !known || seenComponents[component] {
				return fmt.Errorf("native eval case %s has unknown or duplicate component", evalCase.ID)
			}
			seenComponents[component] = true
		}
		if evalCase.ForbiddenCapabilities == nil {
			return fmt.Errorf("native eval case %s has null forbidden capabilities", evalCase.ID)
		}
		seenCapabilities := make(map[string]bool, len(evalCase.ForbiddenCapabilities))
		for _, capability := range evalCase.ForbiddenCapabilities {
			if !knownCapabilities[capability] || seenCapabilities[capability] {
				return fmt.Errorf("native eval case %s has unknown or duplicate forbidden capability", evalCase.ID)
			}
			seenCapabilities[capability] = true
		}
		decision, err := selector.Select(evalCase.Prompt, generation.TargetNative)
		if err != nil || decision.Runtime != RuntimeNative {
			return fmt.Errorf("native eval case %s is not forced-native compatible", evalCase.ID)
		}
	}
	for category, expected := range nativeEvalCategoryCounts {
		if categoryCounts[category] != expected {
			return fmt.Errorf("native eval category %s count = %d, want %d", category, categoryCounts[category], expected)
		}
	}
	return nil
}

func cloneNativeEvalCases(cases []nativeEvalCase) []nativeEvalCase {
	cloned := make([]nativeEvalCase, len(cases))
	for index, evalCase := range cases {
		cloned[index] = evalCase
		cloned[index].RequiredComponents = append([]string(nil), evalCase.RequiredComponents...)
		cloned[index].ForbiddenCapabilities = append([]string(nil), evalCase.ForbiddenCapabilities...)
	}
	return cloned
}
