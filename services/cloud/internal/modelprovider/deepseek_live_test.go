package modelprovider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/generation"
	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

const nativeEvalSuccessThreshold = 16

type nativeEvalCase struct {
	ID                    string   `json:"id"`
	Category              string   `json:"category"`
	Prompt                string   `json:"prompt"`
	RequiredComponents    []string `json:"requiredComponents"`
	ForbiddenCapabilities []string `json:"forbiddenCapabilities"`
}

type nativeEvalReport struct {
	Total        int
	Successes    int
	Failures     map[string]int
	Attempts     map[int]int
	InputTokens  int
	OutputTokens int
	Duration     time.Duration
}

func (report nativeEvalReport) passed() bool {
	return report.Total == 20 && report.Successes >= nativeEvalSuccessThreshold
}

type countingProvider struct {
	delegate modelprovider.Provider
	calls    int
}

func (provider *countingProvider) Generate(
	ctx context.Context,
	request modelprovider.Request,
) (modelprovider.Response, error) {
	provider.calls++
	return provider.delegate.Generate(ctx, request)
}

func TestDeepSeekLiveWorkflowRunsBoundedQualityGates(t *testing.T) {
	t.Parallel()

	workflowPath := filepath.Join("..", "..", "..", "..", ".github", "workflows", "deepseek-live.yml")
	content, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, required := range []string{
		"timeout-minutes: 40",
		"-run '^TestDeepSeekLiveNativeCardQualityGate$'",
		"-count=1",
		"-timeout=35m",
		"AGENTCARD_MODEL_API_KEY: ${{ secrets.AGENTCARD_MODEL_API_KEY }}",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("workflow is missing %q", required)
		}
	}
	if strings.Count(source, "go test ./internal/modelprovider") != 1 ||
		strings.Count(source, "go test ./internal/agent") != 1 ||
		strings.Count(source, "${{ secrets.AGENTCARD_MODEL_API_KEY }}") != 2 {
		t.Fatalf("workflow must run two bounded paid suites with one secret env binding each")
	}
	for _, line := range strings.Split(source, "\n") {
		if strings.Contains(line, "secrets.") &&
			!strings.Contains(line, "AGENTCARD_MODEL_API_KEY:") {
			t.Fatalf("workflow references a secret outside its env binding: %q", line)
		}
	}
}

func TestLiveNativeEvalLoaderRejectsStrictFixtureDrift(t *testing.T) {
	t.Parallel()

	valid, err := loadNativeEvalCases()
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
		{name: "category drift", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Category = "unknown"
			return cases
		}},
		{name: "blank prompt", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = " "
			return cases
		}},
		{name: "oversized prompt", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = strings.Repeat("长", 501)
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
		{name: "unknown forbidden capability", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].ForbiddenCapabilities = []string{"filesystem.read"}
			return cases
		}},
		{name: "duplicate forbidden capability", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].ForbiddenCapabilities = []string{"network.fetch", "network.fetch"}
			return cases
		}},
		{name: "not forced native", mutate: func(cases []nativeEvalCase) []nativeEvalCase {
			cases[0].Prompt = "生成一个自由绘制画板"
			return cases
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cases := cloneLiveNativeEvalCases(valid)
			encoded, err := json.Marshal(test.mutate(cases))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeAndValidateNativeEvalCases(bytes.NewReader(encoded)); err == nil {
				t.Fatal("decodeAndValidateNativeEvalCases() error = nil")
			}
		})
	}
	for _, source := range []string{
		`[{"id":"case","category":"timer","prompt":"prompt","requiredComponents":["Text"],"forbiddenCapabilities":[],"unknown":true}]`,
		`[] {}`,
	} {
		if _, err := decodeAndValidateNativeEvalCases(strings.NewReader(source)); err == nil {
			t.Fatalf("decodeAndValidateNativeEvalCases(%q) error = nil", source)
		}
	}
}

func TestNativeEvalReportRequiresSixteenOfTwenty(t *testing.T) {
	t.Parallel()

	if !(nativeEvalReport{Total: 20, Successes: 16}).passed() {
		t.Fatal("16/20 report did not pass")
	}
	for _, report := range []nativeEvalReport{
		{Total: 20, Successes: 15},
		{Total: 19, Successes: 16},
	} {
		if report.passed() {
			t.Fatalf("report unexpectedly passed: %#v", report)
		}
	}
}

func TestValidateNativeEvalOutputRejectsMissingComponentAndForbiddenMethod(t *testing.T) {
	t.Parallel()

	missingComponent := nativeEvalCase{RequiredComponents: []string{"Divider"}}
	if err := validateNativeEvalOutput(universalNativeEvalCard, missingComponent); err == nil {
		t.Fatal("validateNativeEvalOutput() accepted missing required component")
	}
	forbiddenMethod := nativeEvalCase{
		RequiredComponents:    []string{"Button"},
		ForbiddenCapabilities: []string{"network.fetch"},
	}
	content := `{"root":{"type":"Button","events":{"onPressed":[{"method":"network.fetch"}]}}}`
	if err := validateNativeEvalOutput(content, forbiddenMethod); err == nil {
		t.Fatal("validateNativeEvalOutput() accepted forbidden capability method")
	}
}

func TestValidateNativeEvalOutputRejectsUniversalCardWithoutCaseSemantics(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	caseByID := make(map[string]nativeEvalCase, len(cases))
	for _, evalCase := range cases {
		caseByID[evalCase.ID] = evalCase
	}
	for _, id := range []string{
		"timer-pomodoro",
		"todo-daily-checklist",
		"dashboard-project-status",
		"form-contact-note",
		"chart-focus-week",
		"offline-quick-notes",
		"offline-counter",
		"privacy-water-counter",
		"privacy-mood-picker",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if err := validateNativeEvalOutput(universalNativeEvalCard, caseByID[id]); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted a universal component-only card")
			}
		})
	}
}

func TestValidateNativeEvalOutputRejectsRequiredComponentsWithoutCaseSemantics(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	caseByID := make(map[string]nativeEvalCase, len(cases))
	for _, evalCase := range cases {
		caseByID[evalCase.ID] = evalCase
	}
	for _, id := range []string{
		"timer-pomodoro",
		"todo-daily-checklist",
		"dashboard-project-status",
		"form-contact-note",
		"chart-focus-week",
		"offline-quick-notes",
		"offline-counter",
		"privacy-water-counter",
		"privacy-mood-picker",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			evalCase := caseByID[id]
			if err := validateNativeEvalOutput(requiredComponentsOnlyCard(t, evalCase), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted components without required behavior")
			}
		})
	}
}

func TestValidateNativeEvalOutputAllowsFiveSupportingComponentTypesButRejectsSix(t *testing.T) {
	t.Parallel()

	evalCase := loadNativeEvalCasesByID(t)["dashboard-project-status"]
	document := nativeEvalDashboardCard(evalCase)
	root := document["root"].(map[string]any)
	children := root["children"].([]any)
	children = append(children,
		nativeEvalNodeMap("status", "Badge", map[string]any{"label": "On track"}, nil, nil),
		nativeEvalNodeMap("separator", "Divider", nil, nil, nil),
		nativeEvalNodeMap("icon", "Icon", map[string]any{"name": "widgets"}, nil, nil),
	)
	root["children"] = children
	if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err != nil {
		t.Fatalf("validateNativeEvalOutput() rejected five supporting component types: %v", err)
	}

	root["children"] = append(children,
		nativeEvalNodeMap("row", "Row", nil, nil, nil),
	)
	if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
		t.Fatal("validateNativeEvalOutput() accepted six supporting component types")
	}
}

func TestCaseSpecificNativeEvalCardsSatisfyContractAndSemantics(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	validator := agent.NewNativeValidator()
	for _, evalCase := range cases {
		t.Run(evalCase.ID, func(t *testing.T) {
			t.Parallel()
			content := nativeEvalCardForCase(evalCase)
			if err := validator.Validate(content); err != nil {
				t.Fatalf("NativeValidator.Validate() error = %v", err)
			}
			if err := validateNativeEvalOutput(content, evalCase); err != nil {
				t.Fatalf("validateNativeEvalOutput() error = %v", err)
			}
		})
	}
}

func TestValidateNativeEvalOutputRejectsWrongTimerSemanticsByID(t *testing.T) {
	t.Parallel()

	caseByID := loadNativeEvalCasesByID(t)
	for _, id := range []string{"timer-pomodoro", "timer-short-break", "timer-stopwatch"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			evalCase := caseByID[id]
			document := nativeEvalTimerCard(evalCase)
			switch id {
			case "timer-pomodoro":
				document["initialState"].(map[string]any)["remaining"] = 300
			case "timer-short-break":
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["value"] = 0 })
			case "timer-stopwatch":
				mutateNativeEvalAction(document, "startTimer", func(action map[string]any) {
					action["value"].(map[string]any)["delta"] = -1
				})
			}
			if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted timer with wrong initial/reset/direction")
			}
		})
	}
}

func TestValidateNativeEvalOutputRequiresEnoughDashboardMetrics(t *testing.T) {
	t.Parallel()

	caseByID := loadNativeEvalCasesByID(t)
	for _, id := range []string{"dashboard-study-progress", "dashboard-budget-summary"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			evalCase := caseByID[id]
			document := nativeEvalDashboardCard(evalCase)
			root := document["root"].(map[string]any)
			children := root["children"].([]any)
			filtered := make([]any, 0, len(children))
			keptMetric := false
			for _, child := range children {
				node := child.(map[string]any)
				if node["type"] == "KeyValue" {
					if keptMetric {
						continue
					}
					keptMetric = true
				}
				filtered = append(filtered, child)
			}
			root["children"] = filtered
			if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted dashboard with too few metrics")
			}
		})
	}
}

func TestValidateNativeEvalOutputRejectsTimerOnlyFormButton(t *testing.T) {
	t.Parallel()

	evalCase := loadNativeEvalCasesByID(t)["form-contact-note"]
	for _, actionType := range []string{"startTimer", "stopTimer"} {
		t.Run(actionType, func(t *testing.T) {
			t.Parallel()
			document := nativeEvalFormCard(evalCase)
			visitNativeEvalNodeMaps(document["root"].(map[string]any), func(node map[string]any) {
				if node["type"] != "Button" {
					return
				}
				action := map[string]any{"type": actionType, "path": "timer"}
				if actionType == "startTimer" {
					action["value"] = map[string]any{"intervalMs": 1000, "delta": 1}
				}
				node["events"] = map[string]any{"onPressed": []any{action}}
			})
			document["initialState"].(map[string]any)["timer"] = 0
			if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted a form with only timer actions")
			}
		})
	}
}

func TestValidateNativeEvalOutputRejectsExpressionLikeLiteral(t *testing.T) {
	t.Parallel()

	evalCase := loadNativeEvalCasesByID(t)["timer-pomodoro"]
	document := nativeEvalTimerCard(evalCase)
	root := document["root"].(map[string]any)
	visitNativeEvalNodeMaps(root, func(node map[string]any) {
		if node["type"] == "Text" {
			node["props"].(map[string]any)["text"] = "25:00"
		}
	})
	root["children"] = append(root["children"].([]any), nativeEvalNodeMap(
		"metadata",
		"KeyValue",
		map[string]any{
			"label": "Metadata",
			"value": map[string]any{
				"op": "formatDuration", "args": []any{1500}, "kind": "literal",
			},
		},
		nil,
		nil,
	))
	content := marshalNativeEvalTestCard(t, document)
	if err := agent.NewNativeValidator().Validate(content); err != nil {
		t.Fatalf("NativeValidator.Validate() error = %v", err)
	}
	if err := validateNativeEvalOutput(content, evalCase); err == nil {
		t.Fatal("validateNativeEvalOutput() counted an expression-like literal")
	}
}

func TestValidateNativeEvalOutputRequiresExistingAndCoherentStateActionPaths(t *testing.T) {
	t.Parallel()

	caseByID := loadNativeEvalCasesByID(t)
	tests := []struct {
		id     string
		mutate func(map[string]any)
	}{
		{
			id: "offline-counter",
			mutate: func(document map[string]any) {
				document["initialState"].(map[string]any)["unrelated"] = float64(0)
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["path"] = "unrelated" })
			},
		},
		{
			id: "privacy-water-counter",
			mutate: func(document map[string]any) {
				document["initialState"].(map[string]any)["unrelated"] = float64(0)
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["path"] = "unrelated" })
			},
		},
		{
			id: "form-contact-note",
			mutate: func(document map[string]any) {
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["path"] = "missing" })
			},
		},
		{
			id: "offline-quick-notes",
			mutate: func(document map[string]any) {
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["path"] = "missing" })
			},
		},
		{
			id: "privacy-mood-picker",
			mutate: func(document map[string]any) {
				mutateNativeEvalAction(document, "set", func(action map[string]any) { action["path"] = "missing" })
			},
		},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()
			evalCase := caseByID[test.id]
			document := nativeEvalCardDocumentForCase(evalCase)
			test.mutate(document)
			if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted incoherent or missing state action paths")
			}
		})
	}
}

func TestValidateNativeEvalOutputRequiresTodoCheckboxesInsideOneList(t *testing.T) {
	t.Parallel()

	evalCase := loadNativeEvalCasesByID(t)["todo-daily-checklist"]
	valid := nativeEvalTodoCard(evalCase)
	validRoot := valid["root"].(map[string]any)
	var checkboxes []any
	for _, child := range validRoot["children"].([]any) {
		node := child.(map[string]any)
		if node["type"] == "Checkbox" {
			checkboxes = append(checkboxes, child)
		}
	}
	list := nativeEvalNodeMap("list", "List", nil, nil, []any{
		nativeEvalNodeMap("filler-1", "Text", map[string]any{"text": "One"}, nil, nil),
		nativeEvalNodeMap("filler-2", "Text", map[string]any{"text": "Two"}, nil, nil),
		nativeEvalNodeMap("filler-3", "Text", map[string]any{"text": "Three"}, nil, nil),
	})
	valid["root"] = nativeEvalNodeMap("root", "Container", nil, nil, append([]any{list}, checkboxes...))
	if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, valid), evalCase); err == nil {
		t.Fatal("validateNativeEvalOutput() counted List-external checkboxes")
	}
}

func TestValidateNativeEvalOutputRequiresDistinctBooleanTodoPaths(t *testing.T) {
	t.Parallel()

	evalCase := loadNativeEvalCasesByID(t)["todo-daily-checklist"]
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "duplicate valuePath",
			mutate: func(document map[string]any) {
				visitNativeEvalNodeMaps(document["root"].(map[string]any), func(node map[string]any) {
					if node["type"] == "Checkbox" {
						node["props"].(map[string]any)["valuePath"] = "done1"
					}
				})
			},
		},
		{
			name: "non-boolean state",
			mutate: func(document map[string]any) {
				document["initialState"].(map[string]any)["done2"] = "false"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := nativeEvalTodoCard(evalCase)
			test.mutate(document)
			if err := validateNativeEvalOutput(marshalNativeEvalTestCard(t, document), evalCase); err == nil {
				t.Fatal("validateNativeEvalOutput() accepted invalid todo value paths")
			}
		})
	}
}

func loadNativeEvalCasesByID(t *testing.T) map[string]nativeEvalCase {
	t.Helper()
	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	indexed := make(map[string]nativeEvalCase, len(cases))
	for _, evalCase := range cases {
		indexed[evalCase.ID] = evalCase
	}
	return indexed
}

func visitNativeEvalNodeMaps(node map[string]any, visit func(map[string]any)) {
	visit(node)
	children, _ := node["children"].([]any)
	for _, child := range children {
		visitNativeEvalNodeMaps(child.(map[string]any), visit)
	}
}

func mutateNativeEvalAction(document map[string]any, actionType string, mutate func(map[string]any)) {
	visitNativeEvalNodeMaps(document["root"].(map[string]any), func(node map[string]any) {
		events, _ := node["events"].(map[string]any)
		for _, rawActions := range events {
			actions, _ := rawActions.([]any)
			for _, rawAction := range actions {
				action := rawAction.(map[string]any)
				if action["type"] == actionType {
					mutate(action)
				}
			}
		}
	})
}

func marshalNativeEvalTestCard(t *testing.T, document map[string]any) string {
	t.Helper()
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func nativeEvalCardDocumentForCase(evalCase nativeEvalCase) map[string]any {
	switch evalCase.Category {
	case "form":
		return nativeEvalFormCard(evalCase)
	case "offline":
		return nativeEvalOfflineCard(evalCase)
	case "no-network-file-clipboard":
		return nativeEvalPrivacyCard(evalCase)
	default:
		panic("unsupported state-action test category")
	}
}

func requiredComponentsOnlyCard(t *testing.T, evalCase nativeEvalCase) string {
	t.Helper()
	children := make([]any, 0, len(evalCase.RequiredComponents)-1)
	for index, component := range evalCase.RequiredComponents {
		node := map[string]any{"id": fmt.Sprintf("node-%d", index), "type": component}
		if index == 0 {
			continue
		}
		children = append(children, node)
	}
	root := map[string]any{"id": "node-0", "type": evalCase.RequiredComponents[0]}
	if len(children) > 0 {
		root["children"] = children
	}
	content, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"initialState":  map[string]any{},
		"root":          root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestNativeEvalRunnerRunsAllCasesSeriallyWithBoundedCallsAndSafeOutput(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	providerFailure := errors.New("provider-sensitive-marker")
	provider := &evalFakeProvider{
		casesBySession: nativeEvalCasesBySession(cases),
		invalidSessions: map[string]bool{
			"native-eval-timer-pomodoro":    true,
			"native-eval-timer-short-break": true,
			"native-eval-timer-stopwatch":   true,
		},
		failedSessions: map[string]error{
			"native-eval-todo-daily-checklist": providerFailure,
		},
	}
	var lines []string
	report := runNativeEvalSuite(
		context.Background(),
		provider,
		cases,
		func(format string, arguments ...any) {
			lines = append(lines, fmt.Sprintf(format, arguments...))
		},
	)

	if report.Successes != 16 || report.Total != 20 || report.Attempts[3] != 3 || report.Attempts[1] != 17 {
		t.Fatalf("report = %#v", report)
	}
	if report.Failures["validation_failed"] != 3 || report.Failures["provider_error"] != 1 {
		t.Fatalf("failures = %#v", report.Failures)
	}
	if provider.maxActive != 1 || len(provider.callsBySession) != 20 {
		t.Fatalf("provider maxActive=%d sessions=%d", provider.maxActive, len(provider.callsBySession))
	}
	for sessionID, calls := range provider.callsBySession {
		if calls < 1 || calls > 3 {
			t.Fatalf("provider calls for %s = %d", sessionID, calls)
		}
	}
	output := strings.Join(lines, "\n")
	for _, forbidden := range []string{
		cases[0].Prompt,
		universalNativeEvalCard,
		"provider-sensitive-marker",
		"AGENTCARD_MODEL_API_KEY",
		"api.deepseek.com",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("evaluation output leaked %q: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "id=timer-pomodoro") || !strings.Contains(output, "summary success=16 total=20 threshold=16") {
		t.Fatalf("evaluation output is incomplete: %s", output)
	}
	if !strings.Contains(output, "case_detail id=timer-pomodoro detail=validation_missing_field") {
		t.Fatalf("evaluation output is missing safe failure detail: %s", output)
	}
}

func TestNativeEvalRunnerCountsUnstartedCasesAfterSuiteCancellation(t *testing.T) {
	t.Parallel()

	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &evalFakeProvider{}
	report := runNativeEvalSuite(ctx, provider, cases, func(string, ...any) {})

	if report.Total != 20 || report.Successes != 0 || report.Failures["cancelled"] != 20 ||
		report.Attempts[0] != 20 || len(provider.callsBySession) != 0 {
		t.Fatalf("report=%#v provider=%#v", report, provider.callsBySession)
	}
}

func TestDeepSeekLiveNativeCardQualityGate(t *testing.T) {
	if os.Getenv("AGENTCARD_DEEPSEEK_LIVE") != "1" {
		t.Skip("paid 20-case live gate disabled; enable explicitly and run with -count=1")
	}
	environment := map[string]string{
		"AGENTCARD_MODEL_BASE_URL": os.Getenv("AGENTCARD_MODEL_BASE_URL"),
		"AGENTCARD_MODEL_API_KEY":  os.Getenv("AGENTCARD_MODEL_API_KEY"),
		"AGENTCARD_MODEL":          os.Getenv("AGENTCARD_MODEL"),
	}
	provider, err := modelprovider.NewHTTPProviderFromEnvironment(environment)
	if err != nil {
		t.Fatal("model environment configuration is invalid")
	}
	cases, err := loadNativeEvalCases()
	if err != nil {
		t.Fatal("native evaluation fixture is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	report := runNativeEvalSuite(ctx, provider, cases, t.Logf)
	if !report.passed() {
		t.Fatalf(
			"native quality gate failed: success=%d total=%d threshold=%d",
			report.Successes,
			report.Total,
			nativeEvalSuccessThreshold,
		)
	}
}

func loadNativeEvalCases() ([]nativeEvalCase, error) {
	path := filepath.Join("..", "agent", "testdata", "native_eval_cases.v1.json")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open native evaluation fixture")
	}
	defer file.Close()
	return decodeAndValidateNativeEvalCases(file)
}

// Keep the paid loader self-contained: the workflow runs only this package and
// must validate its fixture without depending on production or another test package.
func decodeAndValidateNativeEvalCases(reader io.Reader) ([]nativeEvalCase, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var cases []nativeEvalCase
	if err := decoder.Decode(&cases); err != nil {
		return nil, fmt.Errorf("decode native evaluation fixture")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("native evaluation fixture has trailing JSON")
	}
	if err := validateLiveNativeEvalCases(cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func validateLiveNativeEvalCases(cases []nativeEvalCase) error {
	if len(cases) != 20 {
		return fmt.Errorf("native evaluation fixture must contain 20 cases")
	}
	catalogPath := filepath.Join("..", "..", "..", "..", "contracts", "card", "native-card-catalog.v1.json")
	catalogContent, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("open native evaluation catalog")
	}
	var catalog struct {
		Components        map[string]json.RawMessage `json:"components"`
		CapabilityMethods map[string]struct {
			ManifestCapability *string `json:"manifestCapability"`
		} `json:"capabilityMethods"`
	}
	if err := json.Unmarshal(catalogContent, &catalog); err != nil {
		return fmt.Errorf("decode native evaluation catalog")
	}
	knownCapabilities := make(map[string]bool)
	for _, method := range catalog.CapabilityMethods {
		if method.ManifestCapability != nil {
			knownCapabilities[*method.ManifestCapability] = true
		}
	}
	expectedCategories := map[string]int{
		"timer":                     3,
		"todo":                      4,
		"dashboard":                 3,
		"form":                      3,
		"chart":                     3,
		"offline":                   2,
		"no-network-file-clipboard": 2,
	}
	idPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	seenIDs := make(map[string]bool, len(cases))
	categoryCounts := make(map[string]int, len(expectedCategories))
	selector := agent.Selector{}
	for _, evalCase := range cases {
		if !idPattern.MatchString(evalCase.ID) || seenIDs[evalCase.ID] {
			return fmt.Errorf("native evaluation fixture has invalid or duplicate id")
		}
		seenIDs[evalCase.ID] = true
		if _, exists := expectedCategories[evalCase.Category]; !exists {
			return fmt.Errorf("native evaluation fixture has unknown category")
		}
		categoryCounts[evalCase.Category]++
		if strings.TrimSpace(evalCase.Prompt) == "" || utf8.RuneCountInString(evalCase.Prompt) > 500 {
			return fmt.Errorf("native evaluation fixture has invalid prompt")
		}
		if len(evalCase.RequiredComponents) == 0 {
			return fmt.Errorf("native evaluation fixture has no required component")
		}
		seenComponents := make(map[string]bool, len(evalCase.RequiredComponents))
		for _, component := range evalCase.RequiredComponents {
			if _, known := catalog.Components[component]; !known || seenComponents[component] {
				return fmt.Errorf("native evaluation fixture has unknown or duplicate component")
			}
			seenComponents[component] = true
		}
		if evalCase.ForbiddenCapabilities == nil {
			return fmt.Errorf("native evaluation fixture has null forbidden capabilities")
		}
		seenCapabilities := make(map[string]bool, len(evalCase.ForbiddenCapabilities))
		for _, capability := range evalCase.ForbiddenCapabilities {
			if !knownCapabilities[capability] || seenCapabilities[capability] {
				return fmt.Errorf("native evaluation fixture has unknown or duplicate forbidden capability")
			}
			seenCapabilities[capability] = true
		}
		decision, err := selector.Select(evalCase.Prompt, generation.TargetNative)
		if err != nil || decision.Runtime != agent.RuntimeNative {
			return fmt.Errorf("native evaluation fixture contains a non-native case")
		}
	}
	for category, expected := range expectedCategories {
		if categoryCounts[category] != expected {
			return fmt.Errorf("native evaluation fixture category count is invalid")
		}
	}
	return nil
}

func cloneLiveNativeEvalCases(cases []nativeEvalCase) []nativeEvalCase {
	cloned := make([]nativeEvalCase, len(cases))
	for index, evalCase := range cases {
		cloned[index] = evalCase
		cloned[index].RequiredComponents = append([]string(nil), evalCase.RequiredComponents...)
		cloned[index].ForbiddenCapabilities = append([]string(nil), evalCase.ForbiddenCapabilities...)
	}
	return cloned
}

func runNativeEvalSuite(
	ctx context.Context,
	provider modelprovider.Provider,
	cases []nativeEvalCase,
	logf func(string, ...any),
) nativeEvalReport {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	report := nativeEvalReport{
		Failures: make(map[string]int),
		Attempts: make(map[int]int),
	}
	suiteStartedAt := time.Now()
	validator := agent.NewNativeValidator()
	for _, evalCase := range cases {
		caseStartedAt := time.Now()
		if contextErr := ctx.Err(); contextErr != nil {
			category := contextErrorCategory(contextErr)
			recordNativeEvalCase(&report, logf, evalCase.ID, false, category, 0, 0, 0, time.Since(caseStartedAt))
			continue
		}

		counted := &countingProvider{delegate: provider}
		codingAgent := agent.NewCodingAgent(counted, validator)
		result, err := codingAgent.Generate(ctx, agent.Request{
			SessionID: "native-eval-" + evalCase.ID,
			Requirement: generation.RequirementSnapshot{
				InitialPrompt:       evalCase.Prompt,
				AdditionalMessages:  []generation.Message{},
				Target:              generation.TargetNative,
				Locale:              "zh-CN",
				AllowedCapabilities: []string{},
				ConfirmedAt:         time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			},
		})

		success := false
		category := classifyNativeEvalError(ctx, err)
		detail := classifyNativeEvalFailureDetail(err)
		if counted.calls > 3 || result.Attempts != counted.calls {
			category = "internal_error"
			detail = "attempt_accounting"
		} else if err == nil {
			validatorErr := validator.Validate(result.Content)
			var contractErr error
			if validatorErr == nil {
				contractErr = validateNativeEvalOutput(result.Content, evalCase)
			}
			switch {
			case result.Runtime != agent.RuntimeNative || result.Attempts < 1 || result.Attempts > 3:
				category = "contract_assertion_failed"
				detail = "result_metadata"
			case validatorErr != nil:
				category = "contract_assertion_failed"
				detail = "validator_regression"
			case contractErr != nil:
				category = "contract_assertion_failed"
				detail = classifyNativeEvalContractDetail(contractErr)
			default:
				success = true
				category = "ok"
				detail = ""
			}
		}
		if !success && detail != "" {
			logf("case_detail id=%s detail=%s", evalCase.ID, detail)
		}
		recordNativeEvalCase(
			&report,
			logf,
			evalCase.ID,
			success,
			category,
			counted.calls,
			result.InputTokens,
			result.OutputTokens,
			time.Since(caseStartedAt),
		)
	}
	report.Duration = time.Since(suiteStartedAt)
	logf(
		"summary success=%d total=%d threshold=%d durationMs=%d inputTokens=%d outputTokens=%d",
		report.Successes,
		report.Total,
		nativeEvalSuccessThreshold,
		report.Duration.Milliseconds(),
		report.InputTokens,
		report.OutputTokens,
	)
	logf(
		"attempts 0=%d 1=%d 2=%d 3=%d",
		report.Attempts[0],
		report.Attempts[1],
		report.Attempts[2],
		report.Attempts[3],
	)
	logf(
		"failures cancelled=%d timeout=%d selector_rejected=%d provider_error=%d validation_failed=%d contract_assertion_failed=%d internal_error=%d",
		report.Failures["cancelled"],
		report.Failures["timeout"],
		report.Failures["selector_rejected"],
		report.Failures["provider_error"],
		report.Failures["validation_failed"],
		report.Failures["contract_assertion_failed"],
		report.Failures["internal_error"],
	)
	return report
}

func recordNativeEvalCase(
	report *nativeEvalReport,
	logf func(string, ...any),
	caseID string,
	success bool,
	category string,
	attempts int,
	inputTokens int,
	outputTokens int,
	duration time.Duration,
) {
	status := "failed"
	if success {
		status = "success"
		report.Successes++
	} else {
		report.Failures[category]++
	}
	report.Total++
	report.Attempts[attempts]++
	report.InputTokens += inputTokens
	report.OutputTokens += outputTokens
	logf(
		"case id=%s status=%s category=%s attempts=%d durationMs=%d inputTokens=%d outputTokens=%d",
		caseID,
		status,
		category,
		attempts,
		duration.Milliseconds(),
		inputTokens,
		outputTokens,
	)
}

func classifyNativeEvalError(ctx context.Context, err error) string {
	if err == nil {
		return "internal_error"
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErrorCategory(contextErr)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, agent.ErrForbiddenRequirement) || errors.Is(err, agent.ErrUnsupportedRequirement) {
		return "selector_rejected"
	}
	if errors.Is(err, agent.ErrValidationFailed) {
		return "validation_failed"
	}
	return "provider_error"
}

func classifyNativeEvalFailureDetail(err error) string {
	if err == nil {
		return ""
	}
	if modelprovider.IsRetryable(err) {
		return "provider_retryable"
	}
	if !errors.Is(err, agent.ErrValidationFailed) {
		return "provider_permanent"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, " has unknown prop "):
		return "validation_unknown_prop"
	case strings.Contains(message, "unknown NativeCard component"):
		return "validation_unknown_component"
	case strings.Contains(message, "unknown NativeCard action"):
		return "validation_unknown_action"
	case strings.Contains(message, "action ") && strings.Contains(message, " is missing fields:"):
		return "validation_action_missing_field"
	case strings.Contains(message, "action ") && strings.Contains(message, " has unknown fields:"):
		return "validation_action_unknown_field"
	case strings.Contains(message, "action ") && strings.Contains(message, " path is invalid"):
		return "validation_action_path"
	case strings.Contains(message, "missing from initialState"),
		strings.Contains(message, "no writable parent in initialState"),
		strings.Contains(message, "incompatible initialState type"):
		return "validation_state_path"
	case strings.Contains(message, " is missing fields:"), strings.Contains(message, "requires prop"):
		return "validation_missing_field"
	case strings.Contains(message, "expression"), strings.Contains(message, "bindings"):
		return "validation_expression"
	case strings.Contains(message, "does not support event"),
		strings.Contains(message, "events must be an object"):
		return "validation_event_contract"
	case strings.Contains(message, "value must be"),
		strings.Contains(message, "outside the catalog enum"),
		strings.Contains(message, "required pattern"):
		return "validation_value_type"
	case strings.Contains(message, "decode NativeCard"),
		strings.Contains(message, "NativeCard must"):
		return "validation_json_shape"
	default:
		return "validation_other"
	}
}

func classifyNativeEvalContractDetail(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "required component is missing":
		return "contract_required_component"
	case "native evaluation output has excessive component variety":
		return "contract_component_variety"
	case "forbidden capability is present":
		return "contract_forbidden_capability"
	case "required case semantics are missing":
		return "contract_semantics"
	default:
		return "contract_other"
	}
}

func contextErrorCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "cancelled"
}

func validateNativeEvalOutput(content string, evalCase nativeEvalCase) error {
	var document nativeEvalDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return fmt.Errorf("decode native evaluation output")
	}
	semantics := nativeEvalSemantics{
		components:  make(map[string]bool),
		nodes:       make(map[string][]nativeEvalNode),
		actions:     make([]nativeEvalAction, 0),
		methods:     make(map[string]bool),
		expressions: make(map[string]int),
	}
	collectNativeEvalNode(document.Root, &semantics)
	requiredComponents := make(map[string]bool, len(evalCase.RequiredComponents))
	for _, component := range evalCase.RequiredComponents {
		requiredComponents[component] = true
		if !semantics.components[component] {
			return fmt.Errorf("required component is missing")
		}
	}
	if len(semantics.components) > len(requiredComponents)+5 {
		return fmt.Errorf("native evaluation output has excessive component variety")
	}
	for _, capability := range evalCase.ForbiddenCapabilities {
		if semantics.methods[capability] {
			return fmt.Errorf("forbidden capability is present")
		}
	}
	if !hasNativeEvalCaseSemantics(document, semantics, evalCase) {
		return fmt.Errorf("required case semantics are missing")
	}
	return nil
}

type nativeEvalDocument struct {
	InitialState map[string]any `json:"initialState"`
	Root         nativeEvalNode `json:"root"`
}

type nativeEvalNode struct {
	Type     string                        `json:"type"`
	Props    map[string]any                `json:"props"`
	Events   map[string][]nativeEvalAction `json:"events"`
	Children []nativeEvalNode              `json:"children"`
}

type nativeEvalAction struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Method string `json:"method"`
	Value  any    `json:"value"`
}

type nativeEvalSemantics struct {
	components  map[string]bool
	nodes       map[string][]nativeEvalNode
	actions     []nativeEvalAction
	methods     map[string]bool
	expressions map[string]int
}

func collectNativeEvalNode(node nativeEvalNode, semantics *nativeEvalSemantics) {
	semantics.components[node.Type] = true
	semantics.nodes[node.Type] = append(semantics.nodes[node.Type], node)
	collectNativeEvalExpressions(node.Props, semantics.expressions)
	for _, actions := range node.Events {
		for _, action := range actions {
			semantics.actions = append(semantics.actions, action)
			if action.Method != "" {
				semantics.methods[action.Method] = true
			}
			collectNativeEvalExpressions(action.Value, semantics.expressions)
		}
	}
	for _, child := range node.Children {
		collectNativeEvalNode(child, semantics)
	}
}

func collectNativeEvalExpressions(value any, operations map[string]int) {
	switch value := value.(type) {
	case map[string]any:
		operation, operationOK := value["op"].(string)
		_, argumentsOK := value["args"].([]any)
		if len(value) == 2 && operationOK && argumentsOK {
			operations[operation]++
		}
		for _, child := range value {
			collectNativeEvalExpressions(child, operations)
		}
	case []any:
		for _, child := range value {
			collectNativeEvalExpressions(child, operations)
		}
	}
}

func hasNativeEvalCaseSemantics(
	document nativeEvalDocument,
	semantics nativeEvalSemantics,
	evalCase nativeEvalCase,
) bool {
	if !hasNativeEvalStateActionPaths(document.InitialState, semantics.actions) {
		return false
	}
	switch evalCase.Category {
	case "timer":
		return hasNativeEvalTimerSemantics(document, semantics, evalCase)
	case "todo":
		expectedCheckboxes := 3
		if evalCase.ID == "todo-shopping-list" || evalCase.ID == "todo-priority-board" {
			expectedCheckboxes = 4
		}
		return hasNativeEvalTodoList(document.InitialState, semantics.nodes["List"], expectedCheckboxes)
	case "dashboard":
		if !hasNativeEvalProp(semantics.nodes["Progress"], "value") ||
			!hasNativeEvalProps(semantics.nodes["KeyValue"], "label", "value") {
			return false
		}
		switch evalCase.ID {
		case "dashboard-study-progress":
			return countNativeEvalNodesWithProps(semantics.nodes["KeyValue"], "label", "value") >= 2
		case "dashboard-budget-summary":
			return countNativeEvalNodesWithProps(semantics.nodes["KeyValue"], "label", "value") >= 3
		case "dashboard-project-status":
			return hasNativeEvalNonEmptyList(semantics, 3)
		default:
			return false
		}
	case "form":
		return hasNativeEvalFormInputs(semantics) && hasNativeEvalButtonStateAction(semantics)
	case "chart":
		return hasNativeEvalChartValues(document.InitialState, semantics.nodes["Chart"])
	case "offline":
		if evalCase.ID == "offline-quick-notes" {
			return hasNativeEvalValuePaths(semantics.nodes["TextInput"], 1) &&
				hasNativeEvalButtonAction(semantics, "set")
		}
		return hasNativeEvalIncrementAndResetSamePath(semantics)
	case "no-network-file-clipboard":
		if evalCase.ID == "privacy-water-counter" {
			return hasNativeEvalIncrementAndResetSamePath(semantics) &&
				hasNativeEvalProp(semantics.nodes["Progress"], "value")
		}
		return hasNativeEvalSelectOptions(semantics.nodes["Select"]) &&
			hasNativeEvalButtonStateAction(semantics) && len(semantics.nodes["Badge"]) > 0
	default:
		return false
	}
}

func isNativeEvalStateAction(actionType string) bool {
	switch actionType {
	case "set", "increment", "toggle", "append", "remove", "startTimer", "stopTimer":
		return true
	default:
		return false
	}
}

func hasNativeEvalStateActionPaths(initialState map[string]any, actions []nativeEvalAction) bool {
	for _, action := range actions {
		if isNativeEvalStateAction(action.Type) && !hasInitialStatePath(initialState, action.Path) {
			return false
		}
	}
	return true
}

func hasInitialStatePath(initialState map[string]any, path string) bool {
	if path == "" {
		return false
	}
	var current any = initialState
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok || segment == "" {
			return false
		}
		current, ok = object[segment]
		if !ok {
			return false
		}
	}
	return true
}

func hasNativeEvalIncrementAndResetSamePath(semantics nativeEvalSemantics) bool {
	for _, increment := range nativeEvalActionsByType(semantics.nodes["Button"], "increment") {
		if increment.Path != "" &&
			hasNativeEvalAction(semantics.nodes["Button"], "set", increment.Path, float64(0)) {
			return true
		}
	}
	return false
}

func hasNativeEvalTodoList(initialState map[string]any, lists []nativeEvalNode, expected int) bool {
	for _, list := range lists {
		checkboxes := nativeEvalDescendantCheckboxes(list)
		if len(checkboxes) != expected {
			continue
		}
		seenPaths := make(map[string]bool, expected)
		valid := true
		for _, checkbox := range checkboxes {
			path, ok := checkbox.Props["valuePath"].(string)
			if !ok || strings.TrimSpace(path) == "" || seenPaths[path] {
				valid = false
				break
			}
			seenPaths[path] = true
			if _, ok := readNativeEvalStatePath(initialState, path).(bool); !ok {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func nativeEvalDescendantCheckboxes(node nativeEvalNode) []nativeEvalNode {
	var checkboxes []nativeEvalNode
	for _, child := range node.Children {
		if child.Type == "Checkbox" {
			checkboxes = append(checkboxes, child)
		}
		checkboxes = append(checkboxes, nativeEvalDescendantCheckboxes(child)...)
	}
	return checkboxes
}

func hasNativeEvalTimerSemantics(
	document nativeEvalDocument,
	semantics nativeEvalSemantics,
	evalCase nativeEvalCase,
) bool {
	expectedReset := float64(0)
	countdown := false
	switch evalCase.ID {
	case "timer-pomodoro":
		expectedReset = 1500
		countdown = true
	case "timer-short-break":
		expectedReset = 300
		countdown = true
	case "timer-stopwatch":
	default:
		return false
	}
	for _, start := range nativeEvalActionsByType(semantics.nodes["Button"], "startTimer") {
		configuration, ok := start.Value.(map[string]any)
		if !ok || start.Path == "" {
			continue
		}
		delta, deltaOK := configuration["delta"].(float64)
		if !deltaOK || (countdown && delta >= 0) || (!countdown && delta <= 0) {
			continue
		}
		if countdown {
			stopAt, ok := configuration["stopAt"].(float64)
			if !ok || stopAt != 0 {
				continue
			}
		}
		initial, ok := readNativeEvalStatePath(document.InitialState, start.Path).(float64)
		if !ok || initial != expectedReset ||
			!hasNativeEvalAction(semantics.nodes["Button"], "stopTimer", start.Path, nil) ||
			!hasNativeEvalAction(semantics.nodes["Button"], "set", start.Path, expectedReset) {
			continue
		}
		return semantics.expressions["formatDuration"] > 0
	}
	return false
}

func nativeEvalActionsByType(nodes []nativeEvalNode, actionType string) []nativeEvalAction {
	var matched []nativeEvalAction
	for _, node := range nodes {
		for _, actions := range node.Events {
			for _, action := range actions {
				if action.Type == actionType {
					matched = append(matched, action)
				}
			}
		}
	}
	return matched
}

func hasNativeEvalAction(
	nodes []nativeEvalNode,
	actionType string,
	path string,
	expectedValue any,
) bool {
	for _, action := range nativeEvalActionsByType(nodes, actionType) {
		if action.Path == path && (expectedValue == nil || action.Value == expectedValue) {
			return true
		}
	}
	return false
}

func hasNativeEvalNonEmptyList(semantics nativeEvalSemantics, minimum int) bool {
	for _, node := range semantics.nodes["List"] {
		if len(node.Children) >= minimum {
			return true
		}
	}
	return false
}

func hasNativeEvalValuePaths(nodes []nativeEvalNode, minimum int) bool {
	count := 0
	for _, node := range nodes {
		if path, ok := node.Props["valuePath"].(string); ok && strings.TrimSpace(path) != "" {
			count++
		}
	}
	return count >= minimum
}

func hasNativeEvalProp(nodes []nativeEvalNode, name string) bool {
	for _, node := range nodes {
		if _, exists := node.Props[name]; exists {
			return true
		}
	}
	return false
}

func hasNativeEvalProps(nodes []nativeEvalNode, names ...string) bool {
	return countNativeEvalNodesWithProps(nodes, names...) > 0
}

func countNativeEvalNodesWithProps(nodes []nativeEvalNode, names ...string) int {
	count := 0
	for _, node := range nodes {
		valid := true
		for _, name := range names {
			if _, exists := node.Props[name]; !exists {
				valid = false
			}
		}
		if valid {
			count++
		}
	}
	return count
}

func hasNativeEvalFormInputs(semantics nativeEvalSemantics) bool {
	inputCount := 0
	for _, component := range []string{"TextInput", "Checkbox", "Select", "Slider"} {
		nodes := semantics.nodes[component]
		inputCount += len(nodes)
		if len(nodes) > 0 && !hasNativeEvalValuePaths(nodes, len(nodes)) {
			return false
		}
	}
	return inputCount > 0
}

func hasNativeEvalButtonStateAction(semantics nativeEvalSemantics) bool {
	for _, actionType := range []string{"set", "toggle", "increment", "append", "remove"} {
		if hasNativeEvalButtonAction(semantics, actionType) {
			return true
		}
	}
	return false
}

func hasNativeEvalButtonAction(semantics nativeEvalSemantics, actionType string) bool {
	for _, node := range semantics.nodes["Button"] {
		for _, actions := range node.Events {
			for _, action := range actions {
				if action.Type == actionType {
					return true
				}
			}
		}
	}
	return false
}

func hasNativeEvalChartValues(initialState map[string]any, charts []nativeEvalNode) bool {
	for _, chart := range charts {
		value := chart.Props["values"]
		if binding, ok := value.(map[string]any); ok {
			path, _ := binding["path"].(string)
			value = readNativeEvalStatePath(initialState, strings.TrimPrefix(path, "state."))
		}
		items, ok := value.([]any)
		if !ok || len(items) == 0 {
			continue
		}
		numeric := true
		for _, item := range items {
			if _, ok := item.(float64); !ok {
				numeric = false
			}
		}
		if numeric {
			return true
		}
	}
	return false
}

func readNativeEvalStatePath(state map[string]any, path string) any {
	var current any = state
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok || segment == "" {
			return nil
		}
		current = object[segment]
	}
	return current
}

func hasNativeEvalSelectOptions(nodes []nativeEvalNode) bool {
	for _, node := range nodes {
		options, ok := node.Props["options"].([]any)
		if ok && len(options) > 0 {
			return true
		}
	}
	return false
}

type evalFakeProvider struct {
	mu              sync.Mutex
	casesBySession  map[string]nativeEvalCase
	invalidSessions map[string]bool
	failedSessions  map[string]error
	callsBySession  map[string]int
	active          int
	maxActive       int
}

func (provider *evalFakeProvider) Generate(
	_ context.Context,
	request modelprovider.Request,
) (modelprovider.Response, error) {
	sessionID := strings.TrimPrefix(strings.SplitN(request.UserPrompt, "\n", 2)[0], "Session ID: ")
	provider.mu.Lock()
	if provider.callsBySession == nil {
		provider.callsBySession = make(map[string]int)
	}
	provider.callsBySession[sessionID]++
	provider.active++
	if provider.active > provider.maxActive {
		provider.maxActive = provider.active
	}
	provider.mu.Unlock()
	defer func() {
		provider.mu.Lock()
		provider.active--
		provider.mu.Unlock()
	}()
	time.Sleep(time.Millisecond)

	if err := provider.failedSessions[sessionID]; err != nil {
		return modelprovider.Response{}, err
	}
	evalCase, exists := provider.casesBySession[sessionID]
	if !exists {
		return modelprovider.Response{}, fmt.Errorf("unknown native evaluation session")
	}
	content := nativeEvalCardForCase(evalCase)
	if provider.invalidSessions[sessionID] {
		content = `{}`
	}
	return modelprovider.Response{
		Content:      content,
		InputTokens:  2,
		OutputTokens: 3,
	}, nil
}

func nativeEvalCasesBySession(cases []nativeEvalCase) map[string]nativeEvalCase {
	indexed := make(map[string]nativeEvalCase, len(cases))
	for _, evalCase := range cases {
		indexed["native-eval-"+evalCase.ID] = evalCase
	}
	return indexed
}

func nativeEvalCardForCase(evalCase nativeEvalCase) string {
	var document map[string]any
	switch evalCase.Category {
	case "timer":
		document = nativeEvalTimerCard(evalCase)
	case "todo":
		document = nativeEvalTodoCard(evalCase)
	case "dashboard":
		document = nativeEvalDashboardCard(evalCase)
	case "form":
		document = nativeEvalFormCard(evalCase)
	case "chart":
		document = nativeEvalChartCard(evalCase)
	case "offline":
		document = nativeEvalOfflineCard(evalCase)
	case "no-network-file-clipboard":
		document = nativeEvalPrivacyCard(evalCase)
	default:
		panic("unsupported native evaluation category")
	}
	content, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(content)
}

func nativeEvalTimerCard(evalCase nativeEvalCase) map[string]any {
	rootType := "Container"
	initial := 0
	delta := 1
	startValue := map[string]any{"intervalMs": 1000, "delta": delta}
	if nativeEvalCaseRequires(evalCase, "Column") {
		rootType = "Column"
	}
	switch evalCase.ID {
	case "timer-pomodoro":
		initial = 1500
		delta = -1
	case "timer-short-break":
		initial = 300
		delta = -1
	}
	startValue["delta"] = delta
	if delta < 0 {
		startValue["stopAt"] = 0
	}
	return nativeEvalDocumentMap(
		map[string]any{"remaining": initial},
		nativeEvalNodeMap("root", rootType, nil, nil, []any{
			nativeEvalNodeMap("time", "Text", map[string]any{
				"text": map[string]any{"expr": map[string]any{
					"op": "formatDuration", "args": []any{map[string]any{"path": "state.remaining"}},
				}},
			}, nil, nil),
			nativeEvalButtonMap("start", map[string]any{
				"type": "startTimer", "path": "remaining",
				"value": startValue,
			}),
			nativeEvalButtonMap("pause", map[string]any{"type": "stopTimer", "path": "remaining"}),
			nativeEvalButtonMap("reset", map[string]any{"type": "set", "path": "remaining", "value": initial}),
		}),
	)
}

func nativeEvalTodoCard(evalCase nativeEvalCase) map[string]any {
	checkboxCount := 3
	if evalCase.ID == "todo-shopping-list" || evalCase.ID == "todo-priority-board" {
		checkboxCount = 4
	}
	state := make(map[string]any, checkboxCount+1)
	children := make([]any, 0, checkboxCount+2)
	if nativeEvalCaseRequires(evalCase, "Text") {
		children = append(children, nativeEvalNodeMap("title", "Text", map[string]any{"text": "Tasks"}, nil, nil))
	}
	for index := 0; index < checkboxCount; index++ {
		path := fmt.Sprintf("done%d", index+1)
		state[path] = false
		children = append(children, nativeEvalNodeMap(
			fmt.Sprintf("task-%d", index+1),
			"Checkbox",
			map[string]any{"label": fmt.Sprintf("Task %d", index+1), "valuePath": path},
			nil,
			nil,
		))
	}
	if nativeEvalCaseRequires(evalCase, "Select") {
		state["priority"] = "medium"
		children = append(children, nativeEvalNodeMap("priority", "Select", map[string]any{
			"valuePath": "priority", "options": []any{"high", "medium", "low"},
		}, nil, nil))
	}
	if nativeEvalCaseRequires(evalCase, "Badge") {
		children = append(children, nativeEvalNodeMap("status", "Badge", map[string]any{"label": "Active"}, nil, nil))
	}
	return nativeEvalDocumentMap(state, nativeEvalNodeMap("root", "List", nil, nil, children))
}

func nativeEvalDashboardCard(evalCase nativeEvalCase) map[string]any {
	metricCount := 1
	if evalCase.ID == "dashboard-study-progress" {
		metricCount = 2
	} else if evalCase.ID == "dashboard-budget-summary" {
		metricCount = 3
	}
	children := make([]any, 0, metricCount+2)
	for index := 0; index < metricCount; index++ {
		children = append(children, nativeEvalNodeMap(
			fmt.Sprintf("metric-%d", index+1),
			"KeyValue",
			map[string]any{"label": fmt.Sprintf("Metric %d", index+1), "value": index + 1},
			nil,
			nil,
		))
	}
	children = append(children, nativeEvalNodeMap(
		"progress", "Progress", map[string]any{"value": map[string]any{"path": "state.progress"}}, nil, nil,
	))
	if nativeEvalCaseRequires(evalCase, "Badge") {
		children = append(children, nativeEvalNodeMap("status", "Badge", map[string]any{"label": "On track"}, nil, nil))
	}
	if nativeEvalCaseRequires(evalCase, "List") {
		milestones := []any{
			nativeEvalNodeMap("milestone-1", "Text", map[string]any{"text": "One"}, nil, nil),
			nativeEvalNodeMap("milestone-2", "Text", map[string]any{"text": "Two"}, nil, nil),
			nativeEvalNodeMap("milestone-3", "Text", map[string]any{"text": "Three"}, nil, nil),
		}
		children = append(children, nativeEvalNodeMap("milestones", "List", nil, nil, milestones))
	}
	return nativeEvalDocumentMap(
		map[string]any{"progress": 0.5},
		nativeEvalNodeMap("root", "Column", nil, nil, children),
	)
}

func nativeEvalFormCard(evalCase nativeEvalCase) map[string]any {
	state := map[string]any{"saved": false}
	children := make([]any, 0, 5)
	switch evalCase.ID {
	case "form-contact-note":
		state["name"] = ""
		state["kind"] = "friend"
		state["important"] = false
		children = append(children,
			nativeEvalNodeMap("name", "TextInput", map[string]any{"label": "Name", "valuePath": "name"}, nil, nil),
			nativeEvalNodeMap("kind", "Select", map[string]any{"valuePath": "kind", "options": []any{"friend", "work"}}, nil, nil),
			nativeEvalNodeMap("important", "Checkbox", map[string]any{"label": "Important", "valuePath": "important"}, nil, nil),
		)
	case "form-feedback":
		state["score"] = 5
		state["comment"] = ""
		children = append(children,
			nativeEvalNodeMap("score", "Slider", map[string]any{"valuePath": "score", "min": 0, "max": 10}, nil, nil),
			nativeEvalNodeMap("comment", "TextInput", map[string]any{"label": "Comment", "valuePath": "comment"}, nil, nil),
			nativeEvalNodeMap("score-text", "Text", map[string]any{"text": "Score"}, nil, nil),
		)
	case "form-preferences":
		state["theme"] = "light"
		state["reminders"] = false
		state["fontSize"] = 1
		children = append(children,
			nativeEvalNodeMap("theme", "Select", map[string]any{"valuePath": "theme", "options": []any{"light", "dark"}}, nil, nil),
			nativeEvalNodeMap("reminders", "Checkbox", map[string]any{"label": "Reminders", "valuePath": "reminders"}, nil, nil),
			nativeEvalNodeMap("font-size", "Slider", map[string]any{"valuePath": "fontSize", "min": 0, "max": 2}, nil, nil),
		)
	}
	children = append(children, nativeEvalButtonMap("save", map[string]any{"type": "set", "path": "saved", "value": true}))
	return nativeEvalDocumentMap(state, nativeEvalNodeMap("root", "Column", nil, nil, children))
}

func nativeEvalChartCard(evalCase nativeEvalCase) map[string]any {
	children := []any{
		nativeEvalNodeMap("chart", "Chart", map[string]any{"values": map[string]any{"path": "state.values"}}, nil, nil),
		nativeEvalNodeMap("summary", "KeyValue", map[string]any{"label": "Total", "value": 6}, nil, nil),
	}
	if nativeEvalCaseRequires(evalCase, "Text") {
		children = append(children, nativeEvalNodeMap("title", "Text", map[string]any{"text": "Statistics"}, nil, nil))
	}
	if nativeEvalCaseRequires(evalCase, "Badge") {
		children = append(children, nativeEvalNodeMap("status", "Badge", map[string]any{"label": "Stable"}, nil, nil))
	}
	return nativeEvalDocumentMap(
		map[string]any{"values": []any{1, 2, 3}},
		nativeEvalNodeMap("root", "Column", nil, nil, children),
	)
}

func nativeEvalOfflineCard(evalCase nativeEvalCase) map[string]any {
	if evalCase.ID == "offline-quick-notes" {
		return nativeEvalDocumentMap(
			map[string]any{"note": "", "saved": false},
			nativeEvalNodeMap("root", "Column", nil, nil, []any{
				nativeEvalNodeMap("note", "TextInput", map[string]any{"label": "Note", "valuePath": "note"}, nil, nil),
				nativeEvalButtonMap("save", map[string]any{"type": "set", "path": "saved", "value": true}),
				nativeEvalNodeMap("status", "Text", map[string]any{"text": "Saved"}, nil, nil),
			}),
		)
	}
	return nativeEvalDocumentMap(
		map[string]any{"count": 0},
		nativeEvalNodeMap("root", "Column", nil, nil, []any{
			nativeEvalNodeMap("count", "Text", map[string]any{
				"text": map[string]any{"expr": map[string]any{"op": "concat", "args": []any{map[string]any{"path": "state.count"}}}},
			}, nil, nil),
			nativeEvalButtonMap("increase", map[string]any{"type": "increment", "path": "count", "value": 1}),
			nativeEvalButtonMap("reset", map[string]any{"type": "set", "path": "count", "value": 0}),
			nativeEvalNodeMap("status", "Badge", map[string]any{"label": "Offline"}, nil, nil),
		}),
	)
}

func nativeEvalPrivacyCard(evalCase nativeEvalCase) map[string]any {
	if evalCase.ID == "privacy-water-counter" {
		return nativeEvalDocumentMap(
			map[string]any{"cups": 0},
			nativeEvalNodeMap("root", "Column", nil, nil, []any{
				nativeEvalNodeMap("cups", "Text", map[string]any{"text": "Cups"}, nil, nil),
				nativeEvalNodeMap("progress", "Progress", map[string]any{"value": map[string]any{"path": "state.cups"}}, nil, nil),
				nativeEvalButtonMap("increase", map[string]any{"type": "increment", "path": "cups", "value": 1}),
				nativeEvalButtonMap("reset", map[string]any{"type": "set", "path": "cups", "value": 0}),
			}),
		)
	}
	return nativeEvalDocumentMap(
		map[string]any{"mood": "happy", "confirmed": false},
		nativeEvalNodeMap("root", "Column", nil, nil, []any{
			nativeEvalNodeMap("mood", "Select", map[string]any{
				"valuePath": "mood", "options": []any{"happy", "calm"},
			}, nil, nil),
			nativeEvalButtonMap("confirm", map[string]any{"type": "set", "path": "confirmed", "value": true}),
			nativeEvalNodeMap("status", "Badge", map[string]any{"label": "Selected"}, nil, nil),
		}),
	)
}

func nativeEvalDocumentMap(initialState map[string]any, root map[string]any) map[string]any {
	return map[string]any{"schemaVersion": 1, "initialState": initialState, "root": root}
}

func nativeEvalNodeMap(
	id string,
	typeName string,
	props map[string]any,
	events map[string]any,
	children []any,
) map[string]any {
	node := map[string]any{"id": id, "type": typeName}
	if len(props) > 0 {
		node["props"] = props
	}
	if len(events) > 0 {
		node["events"] = events
	}
	if len(children) > 0 {
		node["children"] = children
	}
	return node
}

func nativeEvalButtonMap(id string, actions ...map[string]any) map[string]any {
	items := make([]any, len(actions))
	for index, action := range actions {
		items[index] = action
	}
	return nativeEvalNodeMap(id, "Button", nil, map[string]any{"onPressed": items}, nil)
}

func nativeEvalCaseRequires(evalCase nativeEvalCase, component string) bool {
	for _, required := range evalCase.RequiredComponents {
		if required == component {
			return true
		}
	}
	return false
}

const universalNativeEvalCard = `{"schemaVersion":1,"initialState":{"input":"","checked":false,"choice":"a","slider":0},"root":{"id":"root","type":"Container","children":[{"id":"text","type":"Text"},{"id":"button","type":"Button"},{"id":"column","type":"Column"},{"id":"list","type":"List"},{"id":"checkbox","type":"Checkbox","props":{"valuePath":"checked"}},{"id":"select","type":"Select","props":{"valuePath":"choice","options":["a","b"]}},{"id":"key-value","type":"KeyValue"},{"id":"progress","type":"Progress"},{"id":"badge","type":"Badge"},{"id":"text-input","type":"TextInput","props":{"valuePath":"input"}},{"id":"slider","type":"Slider","props":{"valuePath":"slider","min":0,"max":1}},{"id":"chart","type":"Chart"}]}}`
