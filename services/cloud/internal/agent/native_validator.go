package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type NativeValidator struct{}

func NewNativeValidator() NativeValidator {
	return NativeValidator{}
}

func (NativeValidator) Validate(content string, allowedCapabilities ...string) error {
	context, err := loadNativeContractContext()
	if err != nil {
		return err
	}
	if len(content) > context.Catalog.Limits.MaxContentBytes {
		return fmt.Errorf("NativeCard exceeds 2 MiB size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode NativeCard: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("NativeCard must contain one JSON value")
	}

	root, ok := document.(map[string]any)
	if !ok {
		return fmt.Errorf("NativeCard must be an object")
	}
	if err := validateObjectFields(root, setOf("schemaVersion", "initialState", "root"), setOf("schemaVersion", "initialState", "root"), "NativeCard"); err != nil {
		return err
	}
	if version, ok := integerValue(root["schemaVersion"]); !ok || version != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	initialState, ok := root["initialState"].(map[string]any)
	if !ok {
		return fmt.Errorf("initialState must be an object")
	}
	rootNode, ok := root["root"].(map[string]any)
	if !ok {
		return fmt.Errorf("root must be an object")
	}

	capabilities := make(map[string]bool, len(allowedCapabilities))
	for _, capability := range allowedCapabilities {
		capabilities[capability] = true
	}
	state := nativeValidationState{
		catalog:             context.Catalog,
		allowedCapabilities: capabilities,
		seenIDs:             make(map[string]bool),
		statePaths:          make(map[string]any),
	}
	if err := state.consumeInitialState(initialState); err != nil {
		return fmt.Errorf("initialState: %w", err)
	}
	return state.validateNode(rootNode, 1)
}

type nativeValidationState struct {
	catalog             nativeCatalog
	allowedCapabilities map[string]bool
	seenIDs             map[string]bool
	statePaths          map[string]any
	nodes               int
	dataNodes           int
}

func (state *nativeValidationState) validateNode(node map[string]any, depth int) error {
	if depth > state.catalog.Limits.MaxDepth {
		return fmt.Errorf("NativeCard exceeds %d levels", state.catalog.Limits.MaxDepth)
	}
	if err := validateObjectFields(node, setOf("id", "type", "props", "events", "children"), setOf("id", "type"), "node"); err != nil {
		return err
	}
	id, ok := node["id"].(string)
	if !ok || strings.TrimSpace(id) == "" || utf8.RuneCountInString(id) > 100 {
		return fmt.Errorf("node id is invalid")
	}
	if state.seenIDs[id] {
		return fmt.Errorf("duplicate node id %q", id)
	}
	state.seenIDs[id] = true
	typeName, ok := node["type"].(string)
	if !ok {
		return fmt.Errorf("node type must be a string")
	}
	component, ok := state.catalog.Components[typeName]
	if !ok {
		return fmt.Errorf("unknown NativeCard component %q", typeName)
	}

	props := map[string]any{}
	if rawProps, exists := node["props"]; exists {
		var propsOK bool
		props, propsOK = rawProps.(map[string]any)
		if !propsOK {
			return fmt.Errorf("node %q props must be an object", id)
		}
	}
	for name, value := range props {
		rule, allowed := component.AllowedProps[name]
		if !allowed {
			return fmt.Errorf("component %s has unknown prop %q", typeName, name)
		}
		if err := state.validateValue(value, rule, 1); err != nil {
			return fmt.Errorf("component %s prop %s: %w", typeName, name, err)
		}
	}
	for _, required := range component.RequiredProps {
		if _, exists := props[required]; !exists {
			return fmt.Errorf("component %s requires prop %q", typeName, required)
		}
	}
	if rawPath, exists := props["valuePath"]; exists {
		path, _ := rawPath.(string)
		if _, found := state.readState(path); !found {
			return fmt.Errorf("component %s valuePath %q is missing from initialState", typeName, path)
		}
	}
	if typeName == "Slider" {
		minimum := 0.0
		maximum := 1.0
		if value, exists := props["min"]; exists {
			minimum, _ = numberValue(value)
		}
		if value, exists := props["max"]; exists {
			maximum, _ = numberValue(value)
		}
		if minimum > maximum {
			return fmt.Errorf("component Slider requires min <= max")
		}
	}

	if rawEvents, exists := node["events"]; exists {
		events, ok := rawEvents.(map[string]any)
		if !ok {
			return fmt.Errorf("node %q events must be an object", id)
		}
		allowedEvents := setOf(component.AllowedEvents...)
		for eventName, rawActions := range events {
			if !allowedEvents[eventName] {
				return fmt.Errorf("component %s does not support event %q", typeName, eventName)
			}
			actions, ok := rawActions.([]any)
			if !ok {
				return fmt.Errorf("event %s must be an array", eventName)
			}
			if len(actions) > state.catalog.Limits.MaxActionsPerEvent {
				return fmt.Errorf("event %s exceeds %d actions", eventName, state.catalog.Limits.MaxActionsPerEvent)
			}
			for index, rawAction := range actions {
				action, ok := rawAction.(map[string]any)
				if !ok {
					return fmt.Errorf("event %s action %d must be an object", eventName, index)
				}
				if err := state.validateAction(action); err != nil {
					return fmt.Errorf("event %s action %d: %w", eventName, index, err)
				}
			}
		}
	}

	children := []any{}
	if rawChildren, exists := node["children"]; exists {
		var childrenOK bool
		children, childrenOK = rawChildren.([]any)
		if !childrenOK {
			return fmt.Errorf("node %q children must be an array", id)
		}
	}
	if len(children) > state.catalog.Limits.MaxChildrenPerNode {
		return fmt.Errorf("node exceeds %d children", state.catalog.Limits.MaxChildrenPerNode)
	}
	state.nodes++
	if state.nodes > state.catalog.Limits.MaxNodes {
		return fmt.Errorf("NativeCard exceeds %d nodes", state.catalog.Limits.MaxNodes)
	}
	for index, rawChild := range children {
		child, ok := rawChild.(map[string]any)
		if !ok {
			return fmt.Errorf("node %q child %d must be an object", id, index)
		}
		if err := state.validateNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (state *nativeValidationState) validateAction(action map[string]any) error {
	typeName, ok := action["type"].(string)
	if !ok || typeName == "" {
		return fmt.Errorf("action type must be a non-empty string")
	}
	rule, ok := state.catalog.Actions[typeName]
	if !ok {
		return fmt.Errorf("unknown NativeCard action %q", typeName)
	}
	allowed := setOf("type")
	for _, field := range rule.RequiredFields {
		allowed[field] = true
	}
	for _, field := range rule.OptionalFields {
		allowed[field] = true
	}
	if err := validateObjectFields(action, allowed, setOf(append([]string{"type"}, rule.RequiredFields...)...), "action "+typeName); err != nil {
		return err
	}
	var path string
	if rawPath, exists := action["path"]; exists {
		parsedPath, ok := rawPath.(string)
		if !ok || !validRelativeStatePath(parsedPath) {
			return fmt.Errorf("action %s path is invalid", typeName)
		}
		path = parsedPath
	}
	if rawParams, exists := action["params"]; exists {
		if _, ok := rawParams.(map[string]any); !ok {
			return fmt.Errorf("action %s params must be an object", typeName)
		}
		if err := state.consumeDataValue(rawParams, 1); err != nil {
			return fmt.Errorf("action %s params: %w", typeName, err)
		}
	}
	if value, exists := action["value"]; exists && rule.ValueRule != nil {
		if err := state.validateValue(value, *rule.ValueRule, 1); err != nil {
			return fmt.Errorf("action %s value: %w", typeName, err)
		}
	}
	if typeName != "capability.invoke" {
		return state.validateActionTarget(typeName, path)
	}
	method, ok := action["method"].(string)
	if !ok || method == "" {
		return fmt.Errorf("capability.invoke method must be a non-empty string")
	}
	capability, registered := state.catalog.CapabilityMethods[method]
	if !registered {
		return fmt.Errorf("capability method %q is not registered", method)
	}
	if !capability.NativeSupported {
		return fmt.Errorf("capability method %q is unsupported for NativeCard", method)
	}
	if capability.ManifestCapability == nil || !state.allowedCapabilities[*capability.ManifestCapability] {
		return fmt.Errorf("capability %q required by method %q is not allowlisted", valueOrEmpty(capability.ManifestCapability), method)
	}
	if path != "" && !state.writableStatePath(path) {
		return fmt.Errorf("capability result path %q has no writable parent in initialState", path)
	}
	return nil
}

func (state *nativeValidationState) validateValue(value any, rule nativeValueRule, depth int) error {
	if err := state.consumeDataValue(value, 1); err != nil {
		return err
	}
	if rule.Binding {
		resultType, expression, err := state.validateEvaluatedValue(value, depth)
		if err != nil {
			return err
		}
		if expression && resultType != "unknown" && !compatibleValueType(resultType, rule.Type) {
			return fmt.Errorf("expression result type %s does not match %s", resultType, rule.Type)
		}
		if expression {
			return nil
		}
	} else if bindingShape(value) != "" {
		return fmt.Errorf("bindings are not supported")
	}
	if err := validateLiteralValue(value, rule.Type); err != nil {
		return err
	}
	if len(rule.Enum) > 0 {
		text, ok := value.(string)
		if !ok || !setOf(rule.Enum...)[text] {
			return fmt.Errorf("value %v is outside the catalog enum", value)
		}
	}
	if number, ok := numberValue(value); ok {
		if rule.Minimum != nil && number < *rule.Minimum {
			return fmt.Errorf("value is below minimum %v", *rule.Minimum)
		}
		if rule.Maximum != nil && number > *rule.Maximum {
			return fmt.Errorf("value is above maximum %v", *rule.Maximum)
		}
	}
	if text, ok := value.(string); ok {
		if rule.MinLength != nil && utf8.RuneCountInString(text) < *rule.MinLength {
			return fmt.Errorf("value is shorter than %d", *rule.MinLength)
		}
		if rule.Pattern != "" {
			matches, err := regexp.MatchString(rule.Pattern, text)
			if err != nil {
				return fmt.Errorf("catalog pattern is invalid: %w", err)
			}
			if !matches {
				return fmt.Errorf("value does not match required pattern")
			}
		}
	}
	if rule.UniqueItems {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("uniqueItems requires an array value")
		}
		seen := make(map[string]bool, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return fmt.Errorf("uniqueItems requires string array values")
			}
			if seen[text] {
				return fmt.Errorf("array values must be unique")
			}
			seen[text] = true
		}
	}
	return nil
}

func (state *nativeValidationState) validateEvaluatedValue(value any, depth int) (string, bool, error) {
	object, ok := value.(map[string]any)
	shape := bindingShape(value)
	if !ok || shape == "" {
		return literalType(value), false, nil
	}
	if depth > state.catalog.Limits.MaxExpressionDepth {
		return "", true, fmt.Errorf("expression exceeds depth %d", state.catalog.Limits.MaxExpressionDepth)
	}
	if shape == "path" {
		path := object["path"].(string)
		if !validExpressionPath(path) {
			return "", true, fmt.Errorf("expression path is invalid")
		}
		value, found := state.readState(strings.TrimPrefix(path, "state."))
		if !found {
			return "", true, fmt.Errorf("expression path %q is missing from initialState", path)
		}
		return literalType(value), true, nil
	}
	if shape == "expr" {
		nestedObject := object["expr"].(map[string]any)
		result, err := state.validateExpression(nestedObject, depth+1)
		return result, true, err
	}
	result, err := state.validateExpression(object, depth)
	return result, true, err
}

func (state *nativeValidationState) validateExpression(expression map[string]any, depth int) (string, error) {
	if depth > state.catalog.Limits.MaxExpressionDepth {
		return "", fmt.Errorf("expression exceeds depth %d", state.catalog.Limits.MaxExpressionDepth)
	}
	if err := validateObjectFields(expression, setOf("op", "args"), setOf("op", "args"), "expression"); err != nil {
		return "", err
	}
	operation, ok := expression["op"].(string)
	if !ok || operation == "" {
		return "", fmt.Errorf("expression op must be a non-empty string")
	}
	rule, ok := state.catalog.Expressions[operation]
	if !ok {
		return "", fmt.Errorf("unknown expression operation %q", operation)
	}
	arguments, ok := expression["args"].([]any)
	if !ok {
		return "", fmt.Errorf("expression args must be an array")
	}
	if len(arguments) < rule.MinArgs || (rule.MaxArgs != nil && len(arguments) > *rule.MaxArgs) {
		return "", fmt.Errorf("expression %s has invalid arity", operation)
	}
	for index, argument := range arguments {
		argumentType, _, err := state.validateEvaluatedValue(argument, depth+1)
		if err != nil {
			return "", fmt.Errorf("expression %s argument %d: %w", operation, index, err)
		}
		if rule.ArgumentType != "any" && argumentType != "unknown" && argumentType != rule.ArgumentType {
			return "", fmt.Errorf("expression %s requires %s arguments", operation, rule.ArgumentType)
		}
	}
	if rule.Constraint == "nonNegative" && len(arguments) == 1 {
		if number, ok := state.staticNumber(arguments[0]); ok && number < 0 {
			return "", fmt.Errorf("expression %s requires a non-negative argument", operation)
		}
	}
	if rule.Constraint == "nonZeroDivisor" && len(arguments) == 2 {
		if divisor, ok := state.staticNumber(arguments[1]); ok && divisor == 0 {
			return "", fmt.Errorf("expression %s divisor must be non-zero", operation)
		}
	}
	return rule.ResultType, nil
}

func (state *nativeValidationState) staticNumber(value any) (float64, bool) {
	if number, ok := numberValue(value); ok {
		return number, true
	}
	object, ok := value.(map[string]any)
	if !ok || bindingShape(value) != "path" {
		return 0, false
	}
	path := object["path"].(string)
	if !validExpressionPath(path) {
		return 0, false
	}
	resolved, found := state.readState(strings.TrimPrefix(path, "state."))
	if !found {
		return 0, false
	}
	return numberValue(resolved)
}

func (state *nativeValidationState) consumeInitialState(value map[string]any) error {
	return state.consumeDataValueAt(value, 1, "", true)
}

func (state *nativeValidationState) consumeDataValue(value any, depth int) error {
	return state.consumeDataValueAt(value, depth, "", false)
}

func (state *nativeValidationState) consumeDataValueAt(value any, depth int, path string, indexState bool) error {
	if depth > state.catalog.Limits.MaxDataDepth {
		return fmt.Errorf("JSON data exceeds depth %d", state.catalog.Limits.MaxDataDepth)
	}
	state.dataNodes++
	if state.dataNodes > state.catalog.Limits.MaxDataNodes {
		return fmt.Errorf("JSON data exceeds %d nodes", state.catalog.Limits.MaxDataNodes)
	}
	if indexState && path != "" {
		state.statePaths[path] = value
	}
	switch container := value.(type) {
	case map[string]any:
		if len(container) > state.catalog.Limits.MaxContainerItems {
			return fmt.Errorf("JSON object exceeds %d fields", state.catalog.Limits.MaxContainerItems)
		}
		for key, child := range container {
			childPath := key
			childIndex := indexState && key != "" && !strings.Contains(key, ".")
			if path != "" {
				childPath = path + "." + key
			}
			if err := state.consumeDataValueAt(child, depth+1, childPath, childIndex); err != nil {
				return err
			}
		}
	case []any:
		if len(container) > state.catalog.Limits.MaxContainerItems {
			return fmt.Errorf("JSON array exceeds %d items", state.catalog.Limits.MaxContainerItems)
		}
		for _, child := range container {
			if err := state.consumeDataValueAt(child, depth+1, "", false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state *nativeValidationState) readState(path string) (any, bool) {
	value, found := state.statePaths[path]
	return value, found
}

func (state *nativeValidationState) writableStatePath(path string) bool {
	segments := strings.Split(path, ".")
	if len(segments) == 1 {
		return true
	}
	parent, found := state.readState(strings.Join(segments[:len(segments)-1], "."))
	if !found {
		return false
	}
	_, ok := parent.(map[string]any)
	return ok
}

func (state *nativeValidationState) validateActionTarget(actionType, path string) error {
	if actionType == "set" {
		if !state.writableStatePath(path) {
			return fmt.Errorf("set path %q has no writable parent in initialState", path)
		}
		return nil
	}
	if actionType == "stopTimer" {
		return nil
	}
	target, found := state.readState(path)
	if !found {
		return fmt.Errorf("action %s path %q is missing from initialState", actionType, path)
	}
	valid := false
	switch actionType {
	case "increment", "startTimer":
		_, valid = numberValue(target)
	case "toggle":
		_, valid = target.(bool)
	case "append", "remove":
		_, valid = target.([]any)
	default:
		return nil
	}
	if !valid {
		return fmt.Errorf("action %s path %q has incompatible initialState type", actionType, path)
	}
	return nil
}

func validateLiteralValue(value any, expected string) error {
	valid := false
	switch expected {
	case "any":
		valid = true
	case "string":
		_, valid = value.(string)
	case "number":
		_, valid = numberValue(value)
	case "integer":
		_, valid = integerValue(value)
	case "boolean":
		_, valid = value.(bool)
	case "stringList":
		items, ok := value.([]any)
		valid = ok
		for _, item := range items {
			if _, ok := item.(string); !ok {
				valid = false
			}
		}
	case "numberList":
		items, ok := value.([]any)
		valid = ok
		for _, item := range items {
			if _, ok := numberValue(item); !ok {
				valid = false
			}
		}
	case "object":
		_, valid = value.(map[string]any)
	case "timerConfiguration":
		return validateTimerConfiguration(value)
	default:
		return fmt.Errorf("unknown catalog value type %q", expected)
	}
	if !valid {
		return fmt.Errorf("value must be %s", expected)
	}
	return nil
}

func validateTimerConfiguration(value any) error {
	configuration, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("timer configuration must be an object")
	}
	if err := validateObjectFields(configuration, setOf("intervalMs", "delta", "stopAt"), setOf("intervalMs", "delta"), "timer configuration"); err != nil {
		return err
	}
	interval, ok := integerValue(configuration["intervalMs"])
	if !ok || interval <= 0 {
		return fmt.Errorf("timer intervalMs must be a positive integer")
	}
	if _, ok := numberValue(configuration["delta"]); !ok {
		return fmt.Errorf("timer delta must be numeric")
	}
	if stopAt, exists := configuration["stopAt"]; exists {
		if _, ok := numberValue(stopAt); !ok {
			return fmt.Errorf("timer stopAt must be numeric")
		}
	}
	return nil
}

func validateObjectFields(object map[string]any, allowed, required map[string]bool, context string) error {
	unknown := make([]string, 0)
	for field := range object {
		if !allowed[field] {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%s has unknown fields: %s", context, strings.Join(unknown, ", "))
	}
	missing := make([]string, 0)
	for field := range required {
		if _, exists := object[field]; !exists {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s is missing fields: %s", context, strings.Join(missing, ", "))
	}
	return nil
}

func bindingShape(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if len(object) == 1 {
		if _, ok := object["path"].(string); ok {
			return "path"
		}
		if _, ok := object["expr"].(map[string]any); ok {
			return "expr"
		}
	}
	if len(object) == 2 {
		_, operationOK := object["op"].(string)
		_, argumentsOK := object["args"].([]any)
		if operationOK && argumentsOK {
			return "expression"
		}
	}
	return ""
}

func literalType(value any) string {
	switch value := value.(type) {
	case json.Number:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	case []any:
		if len(value) == 0 {
			return "emptyList"
		}
		numbers := true
		stringsOnly := true
		for _, item := range value {
			if _, ok := item.(json.Number); !ok {
				numbers = false
			}
			if _, ok := item.(string); !ok {
				stringsOnly = false
			}
		}
		if numbers {
			return "numberList"
		}
		if stringsOnly {
			return "stringList"
		}
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func compatibleValueType(actual, expected string) bool {
	return expected == "any" || actual == expected ||
		(actual == "emptyList" && (expected == "numberList" || expected == "stringList"))
}

func validExpressionPath(path string) bool {
	return strings.HasPrefix(path, "state.") && validPathSegments(strings.TrimPrefix(path, "state."))
}

func validRelativeStatePath(path string) bool {
	return validPathSegments(path)
}

func validPathSegments(path string) bool {
	if path == "" {
		return false
	}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return false
		}
	}
	return true
}

func numberValue(value any) (float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Float64()
	return parsed, err == nil
}

func integerValue(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	return parsed, err == nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func setOf(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
