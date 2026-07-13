package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeContractContext(t *testing.T) {
	t.Parallel()

	context, err := loadNativeContractContext()
	if err != nil {
		t.Fatal(err)
	}
	if context.Catalog.Version != 1 || len(context.Catalog.Components) != 22 {
		t.Fatalf("catalog = %#v", context.Catalog)
	}
	if context.Catalog.Limits != (nativeLimits{
		MaxDepth: 32, MaxNodes: 500, MaxChildrenPerNode: 200,
		MaxActionsPerEvent: 64,
		MaxExpressionDepth: 32, MaxDataDepth: 32, MaxDataNodes: 500,
		MaxContainerItems: 200, MaxContentBytes: 2 * 1024 * 1024,
	}) {
		t.Fatalf("limits = %#v", context.Catalog.Limits)
	}
	for name, component := range context.Catalog.Components {
		if name == "Button" {
			if len(component.AllowedEvents) != 1 || component.AllowedEvents[0] != "onPressed" {
				t.Fatalf("Button events = %#v", component.AllowedEvents)
			}
		} else if len(component.AllowedEvents) != 0 {
			t.Fatalf("component %s unexpectedly declares events %#v", name, component.AllowedEvents)
		}
	}
	if required := setOf(context.Catalog.Components["TextInput"].RequiredProps...); !required["valuePath"] {
		t.Fatalf("TextInput required props = %#v", required)
	}
	if required := setOf(context.Catalog.Actions["set"].RequiredFields...); !required["path"] || !required["value"] {
		t.Fatalf("set required fields = %#v", required)
	}
	if _, allowed := setOf(context.Catalog.Actions["toggle"].OptionalFields...)["value"]; allowed {
		t.Fatal("toggle must not allow value")
	}
	if rule := context.Catalog.Actions["startTimer"].ValueRule; rule == nil || rule.Type != "timerConfiguration" {
		t.Fatalf("startTimer value rule = %#v", rule)
	}
	if subtract := context.Catalog.Expressions["subtract"]; subtract.MinArgs != 2 || subtract.MaxArgs == nil || *subtract.MaxArgs != 2 || subtract.ArgumentType != "number" {
		t.Fatalf("subtract rule = %#v", subtract)
	}
	metrics := context.Catalog.CapabilityMethods["system.metrics.get"]
	if !metrics.NativeSupported || metrics.ManifestCapability == nil || *metrics.ManifestCapability != "system.metrics.read" {
		t.Fatalf("system.metrics.get rule = %#v", metrics)
	}
	if storage := context.Catalog.CapabilityMethods["storage.get"]; storage.NativeSupported || storage.UnsupportedReason == "" {
		t.Fatalf("storage.get rule = %#v", storage)
	}
	if context.NativeSchema["$id"] == nil {
		t.Fatal("generated context is missing NativeCard schema")
	}
	if context.LocalRPC.ContractVersion != 1 || len(context.LocalRPC.Methods) != 18 {
		t.Fatalf("local RPC = %#v", context.LocalRPC)
	}
}

func TestGeneratedNativeContractContextIsCurrent(t *testing.T) {
	t.Parallel()

	command := exec.Command("go", "run", "./cmd/native-context-gen", "-check")
	command.Dir = filepath.Join(repoRoot(t), "services", "cloud")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("native-context-gen -check failed: %v\n%s", err, output)
	}
}

func TestNativeContextGeneratorCheckRejectsDrift(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"-output", "-schema-output", "-dart-output", "-typescript-output"} {
		t.Run(flag, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "stale")
			if err := os.WriteFile(outputPath, []byte("stale"), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(
				"go", "run", "./cmd/native-context-gen",
				"-check",
				flag, outputPath,
			)
			command.Dir = filepath.Join(repoRoot(t), "services", "cloud")
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "out of date") {
				t.Fatalf("generator drift check error = %v, output = %q", err, output)
			}
		})
	}
}

func TestNativeContractContextCatalogDeclaresRuntimeSemantics(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "contracts", "card", "native-card-catalog.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Version           int                        `json:"version"`
		Components        map[string]json.RawMessage `json:"components"`
		Actions           map[string]json.RawMessage `json:"actions"`
		Expressions       map[string]json.RawMessage `json:"expressions"`
		CapabilityMethods map[string]json.RawMessage `json:"capabilityMethods"`
		Unsupported       map[string]string          `json:"unsupported"`
	}
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Version != 1 {
		t.Fatalf("version = %d, want 1", catalog.Version)
	}
	if len(catalog.Components) != 22 || len(catalog.Actions) != 8 || len(catalog.Expressions) != 14 {
		t.Fatalf("catalog counts = components:%d actions:%d expressions:%d", len(catalog.Components), len(catalog.Actions), len(catalog.Expressions))
	}
	if len(catalog.CapabilityMethods) != 18 {
		t.Fatalf("capability method count = %d, want 18", len(catalog.CapabilityMethods))
	}
	for _, behavior := range []string{"conditionalVisibility", "dynamicListMapping", "dateFormatting"} {
		if catalog.Unsupported[behavior] == "" {
			t.Fatalf("unsupported behavior %q is undocumented", behavior)
		}
	}
}

func TestNativeCardSchemaDefinesStrictActionsAndExpressions(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "contracts", "card", "native-card.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema has no $defs")
	}
	action, ok := definitions["action"].(map[string]any)
	if !ok {
		t.Fatal("schema has no action definition")
	}
	branches, ok := action["oneOf"].([]any)
	if !ok || len(branches) != 8 {
		t.Fatalf("action oneOf branches = %d, want 8", len(branches))
	}
	for _, name := range []string{"expression", "pathBinding", "expressionBinding", "timerConfiguration"} {
		if _, exists := definitions[name]; !exists {
			t.Fatalf("schema has no %s definition", name)
		}
	}
}

func TestNativeCardSchemaExpressesCatalogComponentAndExpressionRules(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "contracts", "card", "native-card.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatal(err)
	}
	definitions := schema["$defs"].(map[string]any)
	node := definitions["node"].(map[string]any)
	componentBranches, ok := node["oneOf"].([]any)
	if !ok || len(componentBranches) != 22 {
		t.Fatalf("component schema branches = %d, want 22", len(componentBranches))
	}
	textInput := schemaBranch(t, componentBranches, "TextInput")
	textInputProps := textInput["properties"].(map[string]any)["props"].(map[string]any)
	if textInputProps["additionalProperties"] != false || !containsJSONText(textInputProps["required"], "valuePath") {
		t.Fatalf("TextInput props schema = %#v", textInputProps)
	}
	text := schemaBranch(t, componentBranches, "Text")
	textEvents := text["properties"].(map[string]any)["events"].(map[string]any)
	if textEvents["additionalProperties"] != false {
		t.Fatalf("Text events schema = %#v", textEvents)
	}
	button := schemaBranch(t, componentBranches, "Button")
	buttonEvents := button["properties"].(map[string]any)["events"].(map[string]any)["properties"].(map[string]any)
	onPressed, ok := buttonEvents["onPressed"].(map[string]any)
	if !ok || onPressed["maxItems"] != float64(64) {
		t.Fatalf("Button events schema = %#v", buttonEvents)
	}

	expression := definitions["expression"].(map[string]any)
	expressionBranches, ok := expression["oneOf"].([]any)
	if !ok || len(expressionBranches) != 14 {
		t.Fatalf("expression schema branches = %d, want 14", len(expressionBranches))
	}
	subtract := schemaBranch(t, expressionBranches, "subtract")
	args := subtract["properties"].(map[string]any)["args"].(map[string]any)
	if args["minItems"] != float64(2) || args["maxItems"] != float64(2) {
		t.Fatalf("subtract args schema = %#v", args)
	}
}

func TestContractGateChecksNativeContextGeneration(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "tooling", "security", "run-contract-gate.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "go run ./cmd/native-context-gen -check") {
		t.Fatal("contract gate does not check generated NativeCard context")
	}
	if !strings.Contains(string(content), "flutter test test/native_card/native_card_catalog_compatibility_test.dart") {
		t.Fatal("contract gate does not run Dart NativeCard catalog compatibility")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func schemaBranch(t *testing.T, branches []any, name string) map[string]any {
	t.Helper()
	for _, rawBranch := range branches {
		branch := rawBranch.(map[string]any)
		typeRule := branch["properties"].(map[string]any)
		for _, field := range []string{"type", "op"} {
			if rule, ok := typeRule[field].(map[string]any); ok && rule["const"] == name {
				return branch
			}
		}
	}
	t.Fatalf("schema branch %q not found", name)
	return nil
}

func containsJSONText(value any, expected string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}
