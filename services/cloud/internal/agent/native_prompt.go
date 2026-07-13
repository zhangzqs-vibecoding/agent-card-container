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
