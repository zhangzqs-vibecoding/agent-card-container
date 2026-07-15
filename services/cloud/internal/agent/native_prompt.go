package agent

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zzq/agent-card-container/services/cloud/internal/modelprovider"
)

const agentModelMaxTokens = 8192
const maxValidationFeedbackBytes = 2048

func nativeModelRequest(request Request, attempt int, validationFeedback string) modelprovider.Request {
	return modelprovider.Request{
		SystemPrompt: nativeSystemPrompt(),
		UserPrompt: modelUserPrompt(
			request,
			RuntimeNative,
			attempt,
			validationFeedback,
		),
		JSONOutput: true,
		MaxTokens:  agentModelMaxTokens,
	}
}

func webModelRequest(request Request, attempt int, validationFeedback string) modelprovider.Request {
	return modelprovider.Request{
		SystemPrompt: strings.Join([]string{
			"You generate AgentCard CodeCard source using the fixed project template.",
			"The template uses Preact with TypeScript and the automatic JSX runtime; export function Card() from src/card.tsx.",
			`For local runtime APIs, import { agentCard } from "./agentcard"; use only methods represented by Allowed capabilities.`,
			"The card must be self-contained and offline: use only bundled source, local state, and the AgentCard runtime API.",
			"Do not call fetch, XMLHttpRequest, WebSocket, EventSource, Worker, or load remote URLs.",
			"Do not use eval, new Function, dynamic import, inline executable script, or string-to-code techniques.",
			"Do not add dependencies or import packages other than preact, preact/hooks, and the template-owned ./agentcard module.",
			"Return exactly one JSON object whose only top-level field is files.",
			"Only these source paths are allowed: src/card.tsx, src/card.css, src/card.test.tsx.",
			"The files value must map allowed paths to complete UTF-8 source text and must include src/card.tsx.",
			"Do not use Markdown fences or add text outside the JSON object.",
			"The latest validation feedback supersedes the prior invalid answer; regenerate the complete JSON object.",
			`Minimal JSON example: {"files":{"src/card.tsx":"export function Card(){return <main/>}"}}`,
		}, "\n"),
		UserPrompt: modelUserPrompt(
			request,
			RuntimeWeb,
			attempt,
			validationFeedback,
		),
		JSONOutput: true,
		MaxTokens:  agentModelMaxTokens,
	}
}

func nativeSystemPrompt() string {
	return strings.Join([]string{
		"You generate AgentCard NativeCard documents from confirmed requirements.",
		"Return exactly one JSON object and no additional text.",
		"Do not use Markdown fences.",
		"Use only components, props, events, actions, expressions, and capability methods declared by the generated contract context below.",
		"For capability.invoke, use only methods whose nativeSupported field is true and whose manifestCapability appears in the User Prompt Allowed capabilities section.",
		"The latest validation feedback supersedes the prior invalid answer; regenerate the complete JSON object instead of patching or quoting the old answer.",
		"State path rules: valuePath and action.path are relative paths such as form.title and must exactly resolve in initialState; do not prefix them with state.",
		`Only expression path bindings use the state. prefix, for example {"path":"state.form.title"}.`,
		`Exact set action example: {"type":"set","path":"saved","value":true}. Do not add target, payload, args, or state fields to actions.`,
		"Keep component types minimal: use only types needed by the request plus at most three supporting types.",
		`Countdown recipe: wrap the timer in a Container with a Column child; keep numeric seconds in initialState, display it with {"op":"formatDuration","args":[{"path":"state.seconds"}]}, start with {"type":"startTimer","path":"seconds","value":{"intervalMs":1000,"delta":-1,"stopAt":0}}, pause with stopTimer on the same path, and reset with set on the same path to the original seconds. For a stopwatch, initialize/reset to 0, use delta 1, and omit stopAt.`,
		"Static list recipe: place one Checkbox for every requested fixed item inside List.children, give each Checkbox a unique valuePath, and initialize every path to a boolean. List has no props and cannot dynamically map an array.",
		"Dashboard recipe: create a KeyValue with both label and value for each requested metric, give Progress a value, and put every requested fixed milestone inside List.children.",
		`Form recipe: use one consistent state object such as "initialState":{"title":"","priority":"中","done":false,"saved":false}; bind inputs with "valuePath":"title", "valuePath":"priority", and "valuePath":"done"; save with {"type":"set","path":"saved","value":true}; every valuePath and every action.path must use one of those existing paths. TextInput allows label and valuePath; Select allows label, valuePath, and non-empty unique options; Checkbox allows label and valuePath; Slider allows only valuePath, min, and max. Put fixed checklist controls inside List.children.`,
		`Chart recipe: For fixed example data, do not use a state path binding; use a literal such as {"values":[1,2,3]}. Otherwise Chart.props.values may bind to an existing non-empty numeric state array. Use KeyValue or Text for requested summaries.`,
		`Counter recipe: use increment buttons and a set-to-zero reset button on the same numeric initialState path. To display a number in Text, convert it with {"op":"concat","args":[{"path":"state.count"}]}.`,
		"Whenever the requirement asks to show a status, represent a requested status with a Badge instead of plain Text alone. Badge.label should usually be a literal status string; bind it only to a string-valued expression.",
		`Minimal valid JSON example: {"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text"}}`,
		"Generated contract context:",
		nativeContractContextJSON,
	}, "\n")
}

func modelUserPrompt(request Request, runtime Runtime, attempt int, validationFeedback string) string {
	parts := []string{
		"Session ID: " + request.SessionID,
		"Runtime: " + string(runtime),
		"Attempt: " + strconv.Itoa(attempt),
		requirementPrompt(request.Requirement),
	}
	if feedback := stableValidationFeedback(validationFeedback); feedback != "" {
		parts = append(parts,
			"Validation feedback:",
			feedback,
			fmt.Sprintf("Regenerate the complete %s JSON object.", runtime),
		)
	}
	return strings.Join(parts, "\n")
}

func stableValidationFeedback(value string) string {
	normalized := strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, value)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if len(normalized) <= maxValidationFeedbackBytes {
		return normalized
	}
	var result strings.Builder
	result.Grow(maxValidationFeedbackBytes)
	for _, value := range normalized {
		size := utf8.RuneLen(value)
		if result.Len()+size > maxValidationFeedbackBytes {
			break
		}
		result.WriteRune(value)
	}
	return result.String()
}
