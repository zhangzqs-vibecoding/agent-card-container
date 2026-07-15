package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
)

const (
	codeCardEvalTotalCases      = 20
	codeCardEvalSuccesses       = 16
	codeCardEvalCategorySuccess = 3
	codeCardEvalMaxModelCalls   = 60
	codeCardEvalMaxInputTokens  = 300_000
	codeCardEvalMaxOutputTokens = 160_000
	codeCardEvalMaxCostUSD      = 5.0
	codeCardEvalMaxDuration     = 90 * time.Minute
)

var codeCardEvalCategoryCounts = map[string]int{
	"canvas":        4,
	"game":          4,
	"visualization": 4,
	"interaction":   4,
	"offline-tool":  4,
}

type codeCardEvalCase struct {
	ID                  string   `json:"id"`
	Category            string   `json:"category"`
	Prompt              string   `json:"prompt"`
	RequiredSemantics   []string `json:"requiredSemantics"`
	ForbiddenPatterns   []string `json:"forbiddenPatterns"`
	AllowedCapabilities []string `json:"allowedCapabilities"`
	Interactions        []string `json:"interactions"`
}

type codeCardEvalReport struct {
	Total             int
	Successes         int
	CategorySuccesses map[string]int
	ModelCalls        int
	InputTokens       int
	OutputTokens      int
	EstimatedCostUSD  float64
	CostKnown         bool
	Duration          time.Duration
	FailureKinds      map[string]int
}

type codeCardEvalPricing struct {
	InputUSDPerMillion  float64
	OutputUSDPerMillion float64
}

type codeCardEvalGenerator interface {
	Generate(context.Context, Request) (Result, error)
}

func (report codeCardEvalReport) Failures() []string {
	failures := make([]string, 0)
	if report.Total != codeCardEvalTotalCases {
		failures = append(failures, "total")
	}
	if report.Successes < codeCardEvalSuccesses {
		failures = append(failures, "successes")
	}
	for category := range codeCardEvalCategoryCounts {
		if report.CategorySuccesses[category] < codeCardEvalCategorySuccess {
			failures = append(failures, "category:"+category)
		}
	}
	if report.ModelCalls > codeCardEvalMaxModelCalls {
		failures = append(failures, "modelCalls")
	}
	if report.InputTokens > codeCardEvalMaxInputTokens {
		failures = append(failures, "inputTokens")
	}
	if report.OutputTokens > codeCardEvalMaxOutputTokens {
		failures = append(failures, "outputTokens")
	}
	if !report.CostKnown {
		failures = append(failures, "cost:unknown")
	} else if report.EstimatedCostUSD > codeCardEvalMaxCostUSD {
		failures = append(failures, "cost")
	}
	if report.Duration > codeCardEvalMaxDuration {
		failures = append(failures, "duration")
	}
	return failures
}

func (report codeCardEvalReport) clone() codeCardEvalReport {
	cloned := report
	cloned.CategorySuccesses = make(map[string]int, len(report.CategorySuccesses))
	for category, successes := range report.CategorySuccesses {
		cloned.CategorySuccesses[category] = successes
	}
	cloned.FailureKinds = make(map[string]int, len(report.FailureKinds))
	for kind, count := range report.FailureKinds {
		cloned.FailureKinds[kind] = count
	}
	return cloned
}

func runCodeCardEvalSuite(
	ctx context.Context,
	generator codeCardEvalGenerator,
	cases []codeCardEvalCase,
	pricing codeCardEvalPricing,
	logf func(string, ...any),
) codeCardEvalReport {
	startedAt := time.Now()
	report := codeCardEvalReport{
		Total:             len(cases),
		CategorySuccesses: make(map[string]int, len(codeCardEvalCategoryCounts)),
		FailureKinds:      make(map[string]int),
		CostKnown:         pricing.InputUSDPerMillion > 0 && pricing.OutputUSDPerMillion > 0,
	}
	for _, evalCase := range cases {
		caseStartedAt := time.Now()
		caseContext, cancel := context.WithTimeout(ctx, 5*time.Minute)
		result, err := generator.Generate(caseContext, Request{
			SessionID: "codecard-eval-" + evalCase.ID,
			Requirement: generation.RequirementSnapshot{
				InitialPrompt:       evalCase.Prompt,
				Target:              generation.TargetWeb,
				Locale:              "zh-CN",
				AllowedCapabilities: append([]string(nil), evalCase.AllowedCapabilities...),
			},
		})
		cancel()
		report.ModelCalls += result.Attempts
		report.InputTokens += result.InputTokens
		report.OutputTokens += result.OutputTokens
		kind := ""
		switch {
		case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			kind = "cancelled"
		case err != nil:
			kind = "generation_failed"
		case len(result.Files["index.html"]) == 0:
			kind = "invalid_artifact"
		default:
			report.Successes++
			report.CategorySuccesses[evalCase.Category]++
		}
		if kind != "" {
			report.FailureKinds[kind]++
		}
		status := "passed"
		if kind != "" {
			status = "failed"
		}
		logf(
			"case id=%s category=%s status=%s attempts=%d inputTokens=%d outputTokens=%d durationMs=%d errorKind=%s",
			evalCase.ID,
			evalCase.Category,
			status,
			result.Attempts,
			result.InputTokens,
			result.OutputTokens,
			time.Since(caseStartedAt).Milliseconds(),
			kind,
		)
	}
	report.Duration = time.Since(startedAt)
	if report.CostKnown {
		report.EstimatedCostUSD =
			float64(report.InputTokens)/1_000_000*pricing.InputUSDPerMillion +
				float64(report.OutputTokens)/1_000_000*pricing.OutputUSDPerMillion
	}
	logf(
		"summary success=%d total=%d modelCalls=%d inputTokens=%d outputTokens=%d durationMs=%d",
		report.Successes,
		report.Total,
		report.ModelCalls,
		report.InputTokens,
		report.OutputTokens,
		report.Duration.Milliseconds(),
	)
	return report
}

func loadCodeCardEvalCasesFile(path string) ([]codeCardEvalCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CodeCard eval fixture: %w", err)
	}
	defer file.Close()
	return decodeCodeCardEvalCases(file)
}

func decodeCodeCardEvalCases(reader io.Reader) ([]codeCardEvalCase, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var rawCases []json.RawMessage
	if err := decoder.Decode(&rawCases); err != nil {
		return nil, fmt.Errorf("decode CodeCard eval cases: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("CodeCard eval fixture must contain one JSON value")
	}
	requiredFields := []string{
		"id", "category", "prompt", "requiredSemantics", "forbiddenPatterns", "allowedCapabilities", "interactions",
	}
	cases := make([]codeCardEvalCase, 0, len(rawCases))
	for index, rawCase := range rawCases {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawCase, &fields); err != nil {
			return nil, fmt.Errorf("CodeCard eval case %d must be an object", index)
		}
		if len(fields) != len(requiredFields) {
			return nil, fmt.Errorf("CodeCard eval case %d must contain exactly seven fields", index)
		}
		for _, field := range requiredFields {
			if _, exists := fields[field]; !exists {
				return nil, fmt.Errorf("CodeCard eval case %d is missing %s", index, field)
			}
		}
		var evalCase codeCardEvalCase
		caseDecoder := json.NewDecoder(bytes.NewReader(rawCase))
		caseDecoder.DisallowUnknownFields()
		if err := caseDecoder.Decode(&evalCase); err != nil {
			return nil, fmt.Errorf("decode CodeCard eval case %d: %w", index, err)
		}
		cases = append(cases, evalCase)
	}
	return cases, nil
}

func validateCodeCardEvalCases(cases []codeCardEvalCase) error {
	if len(cases) != codeCardEvalTotalCases {
		return fmt.Errorf("CodeCard eval case count = %d, want %d", len(cases), codeCardEvalTotalCases)
	}
	contractContext, err := loadNativeContractContext()
	if err != nil {
		return err
	}
	knownCapabilities := make(map[string]bool)
	for _, method := range contractContext.Catalog.CapabilityMethods {
		if method.ManifestCapability != nil {
			knownCapabilities[*method.ManifestCapability] = true
		}
	}
	idPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	seenIDs := make(map[string]bool, len(cases))
	categoryCounts := make(map[string]int, len(codeCardEvalCategoryCounts))
	for index, evalCase := range cases {
		if !idPattern.MatchString(evalCase.ID) || seenIDs[evalCase.ID] {
			return fmt.Errorf("CodeCard eval case %d has invalid or duplicate id", index)
		}
		seenIDs[evalCase.ID] = true
		if _, exists := codeCardEvalCategoryCounts[evalCase.Category]; !exists {
			return fmt.Errorf("CodeCard eval case %s has unknown category", evalCase.ID)
		}
		categoryCounts[evalCase.Category]++
		if strings.TrimSpace(evalCase.Prompt) == "" || utf8.RuneCountInString(evalCase.Prompt) > 500 {
			return fmt.Errorf("CodeCard eval case %s has invalid prompt", evalCase.ID)
		}
		if err := validateCodeCardEvalStrings(evalCase.ID, "required semantics", evalCase.RequiredSemantics, true); err != nil {
			return err
		}
		if err := validateCodeCardEvalStrings(evalCase.ID, "forbidden patterns", evalCase.ForbiddenPatterns, true); err != nil {
			return err
		}
		if err := validateCodeCardEvalStrings(evalCase.ID, "interactions", evalCase.Interactions, true); err != nil {
			return err
		}
		if evalCase.AllowedCapabilities == nil {
			return fmt.Errorf("CodeCard eval case %s has null allowed capabilities", evalCase.ID)
		}
		seenCapabilities := make(map[string]bool, len(evalCase.AllowedCapabilities))
		for _, capability := range evalCase.AllowedCapabilities {
			if !knownCapabilities[capability] || seenCapabilities[capability] {
				return fmt.Errorf("CodeCard eval case %s has unknown or duplicate capability", evalCase.ID)
			}
			seenCapabilities[capability] = true
		}
	}
	for category, expected := range codeCardEvalCategoryCounts {
		if categoryCounts[category] != expected {
			return fmt.Errorf("CodeCard eval category %s count = %d, want %d", category, categoryCounts[category], expected)
		}
	}
	return nil
}

func validateCodeCardEvalStrings(caseID, field string, values []string, required bool) error {
	if values == nil || (required && len(values) == 0) {
		return fmt.Errorf("CodeCard eval case %s has empty %s", caseID, field)
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 256 || seen[value] {
			return fmt.Errorf("CodeCard eval case %s has invalid or duplicate %s", caseID, field)
		}
		seen[value] = true
	}
	return nil
}

func cloneCodeCardEvalCases(cases []codeCardEvalCase) []codeCardEvalCase {
	cloned := make([]codeCardEvalCase, len(cases))
	for index, evalCase := range cases {
		cloned[index] = evalCase
		cloned[index].RequiredSemantics = append([]string(nil), evalCase.RequiredSemantics...)
		cloned[index].ForbiddenPatterns = append([]string(nil), evalCase.ForbiddenPatterns...)
		cloned[index].AllowedCapabilities = append([]string(nil), evalCase.AllowedCapabilities...)
		cloned[index].Interactions = append([]string(nil), evalCase.Interactions...)
	}
	return cloned
}
