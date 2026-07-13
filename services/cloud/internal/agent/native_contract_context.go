package agent

import (
	"encoding/json"
	"fmt"
	"sync"
)

type nativeContractContext struct {
	Catalog      nativeCatalog          `json:"catalog"`
	NativeSchema map[string]any         `json:"nativeSchema"`
	LocalRPC     nativeLocalRPCContract `json:"localRpc"`
}

type nativeCatalog struct {
	Version           int                             `json:"version"`
	Limits            nativeLimits                    `json:"limits"`
	Components        map[string]nativeComponentRule  `json:"components"`
	Actions           map[string]nativeActionRule     `json:"actions"`
	Expressions       map[string]nativeExpressionRule `json:"expressions"`
	CapabilityMethods map[string]nativeCapabilityRule `json:"capabilityMethods"`
	Unsupported       map[string]string               `json:"unsupported"`
}

type nativeLimits struct {
	MaxDepth           int `json:"maxDepth"`
	MaxNodes           int `json:"maxNodes"`
	MaxChildrenPerNode int `json:"maxChildrenPerNode"`
	MaxActionsPerEvent int `json:"maxActionsPerEvent"`
	MaxExpressionDepth int `json:"maxExpressionDepth"`
	MaxDataDepth       int `json:"maxDataDepth"`
	MaxDataNodes       int `json:"maxDataNodes"`
	MaxContainerItems  int `json:"maxContainerItems"`
	MaxContentBytes    int `json:"maxContentBytes"`
}

type nativeComponentRule struct {
	AllowedProps  map[string]nativeValueRule `json:"allowedProps"`
	RequiredProps []string                   `json:"requiredProps"`
	AllowedEvents []string                   `json:"allowedEvents"`
}

type nativeValueRule struct {
	Type        string   `json:"type"`
	Binding     bool     `json:"binding"`
	Enum        []string `json:"enum"`
	Minimum     *float64 `json:"minimum"`
	Maximum     *float64 `json:"maximum"`
	MinLength   *int     `json:"minLength"`
	Pattern     string   `json:"pattern"`
	UniqueItems bool     `json:"uniqueItems"`
}

type nativeActionRule struct {
	RequiredFields []string         `json:"requiredFields"`
	OptionalFields []string         `json:"optionalFields"`
	ValueRule      *nativeValueRule `json:"valueRule"`
}

type nativeExpressionRule struct {
	MinArgs      int    `json:"minArgs"`
	MaxArgs      *int   `json:"maxArgs"`
	ArgumentType string `json:"argumentType"`
	ResultType   string `json:"resultType"`
	Constraint   string `json:"constraint"`
}

type nativeCapabilityRule struct {
	ManifestCapability *string `json:"manifestCapability"`
	NativeSupported    bool    `json:"nativeSupported"`
	UnsupportedReason  string  `json:"unsupportedReason"`
}

type nativeLocalRPCContract struct {
	ContractVersion int      `json:"contractVersion"`
	Methods         []string `json:"methods"`
	Events          []string `json:"events"`
	StableErrors    []string `json:"stableErrors"`
}

var (
	contractContextOnce sync.Once
	contractContext     nativeContractContext
	contractContextErr  error
)

func loadNativeContractContext() (nativeContractContext, error) {
	contractContextOnce.Do(func() {
		if err := json.Unmarshal([]byte(nativeContractContextJSON), &contractContext); err != nil {
			contractContextErr = fmt.Errorf("decode generated NativeCard context: %w", err)
		}
	})
	return contractContext, contractContextErr
}
