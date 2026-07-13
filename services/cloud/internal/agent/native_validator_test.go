package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeValidatorRejectsCatalogViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "unknown prop",
			content: nativeCard(`{"id":"root","type":"Text","props":{"text":"hello","colour":"red"}}`),
		},
		{
			name:    "TextInput without valuePath",
			content: nativeCard(`{"id":"root","type":"TextInput","props":{"label":"Name"}}`),
		},
		{
			name:    "event on a non-Button component",
			content: nativeCard(`{"id":"root","type":"Text","events":{"onPressed":[]}}`),
		},
		{
			name:    "set without value",
			content: nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"set","path":"title"}]}}`),
		},
		{
			name:    "toggle with value",
			content: nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"toggle","path":"enabled","value":true}]}}`),
		},
		{
			name:    "startTimer with invalid configuration",
			content: nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"startTimer","path":"remaining","value":{"intervalMs":0,"delta":"down"}}]}}`),
		},
		{
			name:    "unknown expression operation",
			content: nativeCard(`{"id":"root","type":"Text","props":{"text":{"expr":{"op":"shell","args":[]}}}}`),
		},
		{
			name:    "wrong expression arity",
			content: nativeCard(`{"id":"root","type":"Text","props":{"text":{"expr":{"op":"subtract","args":[1]}}}}`),
		},
		{
			name:    "malformed expression shape",
			content: nativeCard(`{"id":"root","type":"Text","props":{"text":{"expr":{"op":"add","args":[1],"extra":true}}}}`),
		},
		{
			name:    "unregistered capability method",
			content: nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"capability.invoke","method":"shell.exec"}]}}`),
		},
	}

	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content); err == nil {
				t.Fatal("Validate() error = nil, want catalog violation")
			}
		})
	}
}

func TestNativeValidatorEnforcesCapabilityAllowlist(t *testing.T) {
	t.Parallel()

	validator := NewNativeValidator()
	notification := nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"capability.invoke","method":"notification.show","params":{"body":"done"}}]}}`)
	if err := validator.Validate(notification, "notification.show"); err != nil {
		t.Fatalf("Validate() allowed capability error = %v", err)
	}
	if err := validator.Validate(notification, "storage"); err == nil {
		t.Fatal("Validate() error = nil, want undeclared capability rejection")
	}

	unsupported := nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[{"type":"capability.invoke","method":"storage.get","params":{"key":"name"}}]}}`)
	if err := validator.Validate(unsupported, "storage"); err == nil {
		t.Fatal("Validate() error = nil, want unsupported NativeCard method rejection")
	}
}

func TestNativeValidatorEnforcesActionsPerEventLimit(t *testing.T) {
	t.Parallel()

	const limit = 64
	action := `{"type":"capability.invoke","method":"notification.show"}`
	actions := func(count int) string {
		return strings.TrimSuffix(strings.Repeat(action+",", count), ",")
	}
	validator := NewNativeValidator()
	if err := validator.Validate(nativeCard(actionRoot(actions(limit))), "notification.show"); err != nil {
		t.Fatalf("Validate() exact action limit error = %v", err)
	}
	if err := validator.Validate(nativeCard(actionRoot(actions(limit+1))), "notification.show"); err == nil {
		t.Fatal("Validate() error = nil, want action limit rejection")
	}
}

func TestNativeValidatorRejectsFlutterLayoutAndPathHazards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "empty valuePath", content: nativeCard(`{"id":"root","type":"TextInput","props":{"valuePath":""}}`)},
		{name: "invalid valuePath segment", content: nativeCard(`{"id":"root","type":"Checkbox","props":{"valuePath":"form..done"}}`)},
		{name: "negative padding", content: nativeCard(`{"id":"root","type":"Container","props":{"padding":-1}}`)},
		{name: "negative row spacing", content: nativeCard(`{"id":"root","type":"Row","props":{"spacing":-1}}`)},
		{name: "negative column spacing", content: nativeCard(`{"id":"root","type":"Column","props":{"spacing":-1}}`)},
		{name: "negative grid spacing", content: nativeCard(`{"id":"root","type":"Grid","props":{"spacing":-1}}`)},
		{name: "zero grid columns", content: nativeCard(`{"id":"root","type":"Grid","props":{"columns":0}}`)},
		{name: "too many grid columns", content: nativeCard(`{"id":"root","type":"Grid","props":{"columns":7}}`)},
		{name: "duplicate select options", content: nativeCardWithState(`{"value":"a"}`, `{"id":"root","type":"Select","props":{"valuePath":"value","options":["a","a"]}}`)},
		{name: "slider explicit inverted range", content: nativeCardWithState(`{"value":0}`, `{"id":"root","type":"Slider","props":{"valuePath":"value","min":5,"max":1}}`)},
		{name: "slider min exceeds default max", content: nativeCardWithState(`{"value":0}`, `{"id":"root","type":"Slider","props":{"valuePath":"value","min":2}}`)},
		{name: "slider max below default min", content: nativeCardWithState(`{"value":0}`, `{"id":"root","type":"Slider","props":{"valuePath":"value","max":-1}}`)},
	}
	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content); err == nil {
				t.Fatal("Validate() error = nil, want Flutter runtime hazard rejection")
			}
		})
	}
}

func TestNativeValidatorTreatsNonBindingObjectsAsLiterals(t *testing.T) {
	t.Parallel()

	content := nativeCard(`{"id":"root","type":"Button","events":{"onPressed":[` +
		`{"type":"set","path":"first","value":{"path":"metadata","label":"literal"}},` +
		`{"type":"set","path":"second","value":{"expr":{"label":"literal"},"kind":"data"}},` +
		`{"type":"set","path":"third","value":{"op":"metadata","args":[],"kind":"data"}}` +
		`]}}`)
	if err := NewNativeValidator().Validate(content); err != nil {
		t.Fatalf("Validate() literal object error = %v", err)
	}
}

func TestNativeValidatorRejectsExcessiveJSONData(t *testing.T) {
	t.Parallel()

	deepState := map[string]any{"value": nestedJSONValue(33)}
	tooManyItems := map[string]any{"items": make([]any, 201)}
	totalNodes := map[string]any{
		"first":  make([]any, 170),
		"second": make([]any, 170),
		"third":  make([]any, 170),
	}
	deepParams := nestedJSONValue(33)
	deepLiteral := nestedJSONValue(33)
	tests := []struct {
		name    string
		content string
		caps    []string
	}{
		{name: "deep initialState", content: encodedNativeCard(t, deepState, map[string]any{"id": "root", "type": "Text"})},
		{name: "oversized initialState container", content: encodedNativeCard(t, tooManyItems, map[string]any{"id": "root", "type": "Text"})},
		{name: "excessive total data nodes", content: encodedNativeCard(t, totalNodes, map[string]any{"id": "root", "type": "Text"})},
		{
			name: "deep capability params",
			content: encodedNativeCard(t, map[string]any{}, map[string]any{
				"id": "root", "type": "Button", "events": map[string]any{"onPressed": []any{
					map[string]any{"type": "capability.invoke", "method": "notification.show", "params": deepParams},
				}},
			}),
			caps: []string{"notification.show"},
		},
		{
			name: "deep any literal",
			content: encodedNativeCard(t, map[string]any{}, map[string]any{
				"id": "root", "type": "Button", "events": map[string]any{"onPressed": []any{
					map[string]any{"type": "set", "path": "value", "value": deepLiteral},
				}},
			}),
		},
	}
	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content, test.caps...); err == nil {
				t.Fatal("Validate() error = nil, want JSON data budget rejection")
			}
		})
	}
}

func TestNativeValidatorEnforcesTreeDepthBoundary(t *testing.T) {
	t.Parallel()

	validator := NewNativeValidator()
	if err := validator.Validate(encodedNativeCard(t, map[string]any{}, nestedNativeNodes(32))); err != nil {
		t.Fatalf("Validate() exact tree depth error = %v", err)
	}
	err := validator.Validate(encodedNativeCard(t, map[string]any{}, nestedNativeNodes(33)))
	if err == nil || !strings.Contains(err.Error(), "exceeds 32 levels") {
		t.Fatalf("Validate() over tree depth error = %v, want deterministic depth rejection", err)
	}
}

func TestNativeValidatorValidatesInitialStatePathsAndActionTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		caps    []string
	}{
		{name: "missing display binding", content: nativeCard(`{"id":"root","type":"Text","props":{"text":{"path":"state.missing"}}}`)},
		{name: "missing nested expression binding", content: nativeCard(`{"id":"root","type":"Text","props":{"text":{"expr":{"op":"concat","args":[{"path":"state.missing"}]}}}}`)},
		{name: "missing TextInput valuePath", content: nativeCard(`{"id":"root","type":"TextInput","props":{"valuePath":"missing"}}`)},
		{name: "missing Checkbox valuePath", content: nativeCard(`{"id":"root","type":"Checkbox","props":{"valuePath":"missing"}}`)},
		{name: "missing Select valuePath", content: nativeCard(`{"id":"root","type":"Select","props":{"valuePath":"missing"}}`)},
		{name: "missing Slider valuePath", content: nativeCard(`{"id":"root","type":"Slider","props":{"valuePath":"missing"}}`)},
		{name: "increment wrong target", content: nativeCardWithState(`{"count":"one"}`, actionRoot(`{"type":"increment","path":"count"}`))},
		{name: "toggle wrong target", content: nativeCardWithState(`{"enabled":0}`, actionRoot(`{"type":"toggle","path":"enabled"}`))},
		{name: "append wrong target", content: nativeCardWithState(`{"items":{}}`, actionRoot(`{"type":"append","path":"items","value":1}`))},
		{name: "remove wrong target", content: nativeCardWithState(`{"items":{}}`, actionRoot(`{"type":"remove","path":"items","value":1}`))},
		{name: "startTimer wrong target", content: nativeCardWithState(`{"remaining":"five"}`, actionRoot(`{"type":"startTimer","path":"remaining","value":{"intervalMs":1000,"delta":-1}}`))},
		{name: "set missing writable parent", content: nativeCardWithState(`{}`, actionRoot(`{"type":"set","path":"form.value","value":1}`))},
		{
			name:    "capability result missing writable parent",
			content: nativeCardWithState(`{}`, actionRoot(`{"type":"capability.invoke","method":"notification.show","path":"result.value"}`)),
			caps:    []string{"notification.show"},
		},
	}
	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content, test.caps...); err == nil {
				t.Fatal("Validate() error = nil, want initial-state runtime rejection")
			}
		})
	}

	valid := nativeCardWithState(
		`{"count":1,"enabled":false,"items":[1],"remaining":5,"form":{}}`,
		actionRoot(
			`{"type":"set","path":"newValue","value":1},`+
				`{"type":"set","path":"form.value","value":1},`+
				`{"type":"increment","path":"count"},`+
				`{"type":"toggle","path":"enabled"},`+
				`{"type":"append","path":"items","value":2},`+
				`{"type":"remove","path":"items","value":1},`+
				`{"type":"startTimer","path":"remaining","value":{"intervalMs":1000,"delta":-1}}`,
		),
	)
	if err := validator.Validate(valid); err != nil {
		t.Fatalf("Validate() valid state actions error = %v", err)
	}
}

func TestNativeValidatorAcceptsBoundNumberLists(t *testing.T) {
	t.Parallel()

	validator := NewNativeValidator()
	for _, values := range []string{`[1,2.5]`, `[]`} {
		content := nativeCardWithState(
			`{"values":`+values+`}`,
			`{"id":"chart","type":"Chart","props":{"values":{"path":"state.values"}}}`,
		)
		if err := validator.Validate(content); err != nil {
			t.Fatalf("Validate() bound Chart values %s error = %v", values, err)
		}
	}
}

func TestNativeValidatorRejectsWrongBoundListTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "mixed Chart values",
			content: nativeCardWithState(
				`{"values":[1,"two"]}`,
				`{"id":"chart","type":"Chart","props":{"values":{"path":"state.values"}}}`,
			),
		},
		{
			name: "boolean Chart values",
			content: nativeCardWithState(
				`{"values":[true]}`,
				`{"id":"chart","type":"Chart","props":{"values":{"path":"state.values"}}}`,
			),
		},
		{
			name: "empty list bound to scalar prop",
			content: nativeCardWithState(
				`{"value":[]}`,
				`{"id":"text","type":"Text","props":{"text":{"path":"state.value"}}}`,
			),
		},
	}
	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content); err == nil {
				t.Fatal("Validate() error = nil, want bound list type rejection")
			}
		})
	}
}

func TestLiteralTypeRecognizesLists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "numbers", value: []any{json.Number("1"), json.Number("2.5")}, want: "numberList"},
		{name: "strings", value: []any{"one", "two"}, want: "stringList"},
		{name: "empty", value: []any{}, want: "emptyList"},
		{name: "mixed", value: []any{json.Number("1"), "two"}, want: "array"},
		{name: "unsupported homogeneous", value: []any{true, false}, want: "array"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := literalType(test.value); got != test.want {
				t.Fatalf("literalType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNativeValidatorRejectsInvalidUniqueItemsRuleWithoutPanicking(t *testing.T) {
	t.Parallel()

	state := nativeValidationState{catalog: nativeCatalog{Limits: nativeLimits{
		MaxDataDepth:      4,
		MaxDataNodes:      10,
		MaxContainerItems: 10,
	}}}
	err := state.validateValue(
		[]any{json.Number("1")},
		nativeValueRule{Type: "numberList", UniqueItems: true},
		1,
	)
	if err == nil {
		t.Fatal("validateValue() error = nil, want invalid uniqueItems rule rejection")
	}
}

func TestNativeValidatorRejectsStaticallyKnownExpressionDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "negative formatDuration state value",
			content: nativeCardWithState(
				`{"seconds":-1}`,
				`{"id":"root","type":"Text","props":{"text":{"expr":{"op":"formatDuration","args":[{"path":"state.seconds"}]}}}}`,
			),
		},
		{
			name: "zero divide state value",
			content: nativeCardWithState(
				`{"zero":0}`,
				`{"id":"root","type":"KeyValue","props":{"value":{"expr":{"op":"divide","args":[1,{"path":"state.zero"}]}}}}`,
			),
		},
	}
	validator := NewNativeValidator()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validator.Validate(test.content); err == nil {
				t.Fatal("Validate() error = nil, want static expression domain rejection")
			}
		})
	}
}

func TestNativeValidatorEnforcesContentSizeBeforeDecode(t *testing.T) {
	t.Parallel()

	const limit = 2 * 1024 * 1024
	base := nativeCard(`{"id":"root","type":"Text"}`)
	atLimit := strings.Repeat(" ", limit-len(base)) + base
	validator := NewNativeValidator()
	if err := validator.Validate(atLimit); err != nil {
		t.Fatalf("Validate() exact size limit error = %v", err)
	}
	overLimit := "sensitive-marker" + strings.Repeat(" ", limit+1-len("sensitive-marker")-len(base)) + base
	err := validator.Validate(overLimit)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("Validate() over size error = %v, want size limit", err)
	}
	if strings.Contains(err.Error(), "sensitive-marker") {
		t.Fatalf("Validate() leaked content in error: %v", err)
	}
}

func TestNativeValidatorAcceptsTrustedPomodoroFixture(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "card", "fixtures", "pomodoro-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewNativeValidator().Validate(string(content)); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func nativeCard(root string) string {
	return `{"schemaVersion":1,"initialState":{},"root":` + root + `}`
}

func nativeCardWithState(state, root string) string {
	return `{"schemaVersion":1,"initialState":` + state + `,"root":` + root + `}`
}

func actionRoot(actions string) string {
	return `{"id":"root","type":"Button","events":{"onPressed":[` + actions + `]}}`
}

func nestedJSONValue(depth int) any {
	var value any = "leaf"
	for range depth {
		value = map[string]any{"value": value}
	}
	return value
}

func nestedNativeNodes(depth int) map[string]any {
	var node map[string]any
	for index := depth; index >= 1; index-- {
		next := map[string]any{"id": fmt.Sprintf("node-%d", index), "type": "Container"}
		if node != nil {
			next["children"] = []any{node}
		}
		node = next
	}
	return node
}

func encodedNativeCard(t *testing.T, state map[string]any, root map[string]any) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"initialState":  state,
		"root":          root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
