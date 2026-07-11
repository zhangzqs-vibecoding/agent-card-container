package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type NativeValidator struct{}

func NewNativeValidator() NativeValidator {
	return NativeValidator{}
}

type nativeSpec struct {
	SchemaVersion int            `json:"schemaVersion"`
	InitialState  map[string]any `json:"initialState"`
	Root          nativeNode     `json:"root"`
}

type nativeNode struct {
	ID       string                    `json:"id"`
	Type     string                    `json:"type"`
	Props    map[string]any            `json:"props,omitempty"`
	Events   map[string][]nativeAction `json:"events,omitempty"`
	Children []nativeNode              `json:"children,omitempty"`
}

type nativeAction struct {
	Type   string         `json:"type"`
	Path   string         `json:"path,omitempty"`
	Value  any            `json:"value,omitempty"`
	Method string         `json:"method,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

func (NativeValidator) Validate(content string) error {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var spec nativeSpec
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode NativeCard: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("NativeCard must contain one JSON value")
	}
	if spec.SchemaVersion != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	seen := make(map[string]struct{})
	nodes := 0
	if err := validateNode(spec.Root, 1, &nodes, seen); err != nil {
		return err
	}
	if nodes > 500 {
		return fmt.Errorf("NativeCard exceeds 500 nodes")
	}
	return nil
}

func validateNode(node nativeNode, depth int, nodes *int, seen map[string]struct{}) error {
	if depth > 32 {
		return fmt.Errorf("NativeCard exceeds 32 levels")
	}
	if strings.TrimSpace(node.ID) == "" || len(node.ID) > 100 {
		return fmt.Errorf("node id is invalid")
	}
	if _, exists := seen[node.ID]; exists {
		return fmt.Errorf("duplicate node id %q", node.ID)
	}
	seen[node.ID] = struct{}{}
	if !nativeComponents[node.Type] {
		return fmt.Errorf("unknown NativeCard component %q", node.Type)
	}
	if len(node.Children) > 200 {
		return fmt.Errorf("node exceeds 200 children")
	}
	*nodes++
	if *nodes > 500 {
		return fmt.Errorf("NativeCard exceeds 500 nodes")
	}
	for _, actions := range node.Events {
		for _, action := range actions {
			if !nativeActions[action.Type] {
				return fmt.Errorf("unknown NativeCard action %q", action.Type)
			}
		}
	}
	for _, child := range node.Children {
		if err := validateNode(child, depth+1, nodes, seen); err != nil {
			return err
		}
	}
	return nil
}

var nativeComponents = setOf(
	"Container", "Row", "Column", "Stack", "Grid", "Scroll", "Divider",
	"Text", "Icon", "Image", "Badge", "Progress", "Chart", "Button",
	"TextInput", "Checkbox", "Select", "Slider", "List", "KeyValue",
	"EmptyState", "ErrorState",
)

var nativeActions = setOf(
	"set", "increment", "toggle", "append", "remove", "startTimer",
	"stopTimer", "capability.invoke",
)

func setOf(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
