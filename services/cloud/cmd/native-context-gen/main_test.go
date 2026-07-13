package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSourcesRejectsCatalogSchemaSemanticDrift(t *testing.T) {
	catalog, schema, localRPC := contractSources(t)

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "component",
			mutate: func(document map[string]any) {
				node := schemaDefinition(document, "node")
				node["oneOf"] = node["oneOf"].([]any)[1:]
			},
		},
		{
			name: "action",
			mutate: func(document map[string]any) {
				action := schemaDefinition(document, "action")
				action["oneOf"] = action["oneOf"].([]any)[1:]
			},
		},
		{
			name: "expression",
			mutate: func(document map[string]any) {
				expression := schemaDefinition(document, "expression")
				expression["oneOf"] = expression["oneOf"].([]any)[1:]
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(schema, &document); err != nil {
				t.Fatal(err)
			}
			test.mutate(document)
			mutated, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateSources(catalog, mutated, localRPC); err == nil {
				t.Fatal("validateSources() error = nil, want semantic drift rejection")
			}
		})
	}
}

func TestValidateSourcesRequiresValueRulesForValueActions(t *testing.T) {
	catalog, schema, localRPC := contractSources(t)
	var document map[string]any
	if err := json.Unmarshal(catalog, &document); err != nil {
		t.Fatal(err)
	}
	actions := document["actions"].(map[string]any)
	delete(actions["set"].(map[string]any), "valueRule")
	mutated, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSources(mutated, schema, localRPC); err == nil {
		t.Fatal("validateSources() error = nil, want missing valueRule rejection")
	}
}

func TestGenerateValidatesCatalogAgainstSchema(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "unknown field",
			mutate: func(catalog map[string]any) {
				catalog["surprise"] = true
			},
		},
		{
			name: "missing required field",
			mutate: func(catalog map[string]any) {
				component := catalog["components"].(map[string]any)["Container"].(map[string]any)
				delete(component, "allowedProps")
			},
		},
		{
			name: "wrong type",
			mutate: func(catalog map[string]any) {
				catalog["limits"].(map[string]any)["maxDepth"] = "32"
			},
		},
		{
			name: "invalid enum",
			mutate: func(catalog map[string]any) {
				padding := catalog["components"].(map[string]any)["Container"].(map[string]any)["allowedProps"].(map[string]any)["padding"].(map[string]any)
				padding["type"] = "decimal"
			},
		},
		{
			name: "out of range",
			mutate: func(catalog map[string]any) {
				catalog["limits"].(map[string]any)["maxDepth"] = 0
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := copyContractSources(t)
			path := filepath.Join(root, "contracts", "card", "native-card-catalog.v1.json")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var catalog map[string]any
			if err := json.Unmarshal(content, &catalog); err != nil {
				t.Fatal(err)
			}
			test.mutate(catalog)
			content, err = json.Marshal(catalog)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := generate(root); err == nil {
				t.Fatal("generate() error = nil, want catalog schema rejection")
			}
		})
	}
}

func TestGeneratedNativeSchemaValidatesDocuments(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := generate(root)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compileNativeSchema(artifacts.NativeSchema)
	if err != nil {
		t.Fatalf("compileNativeSchema() error = %v", err)
	}
	trusted, err := os.ReadFile(filepath.Join(root, "contracts", "card", "fixtures", "pomodoro-native.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSchemaDocument(compiled, trusted); err != nil {
		t.Fatalf("trusted fixture validation error = %v", err)
	}

	invalidDocuments := map[string]string{
		"unknown prop":           `{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text","props":{"colour":"red"}}}`,
		"illegal action":         `{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Button","events":{"onPressed":[{"type":"toggle","path":"enabled","value":true}]}}}`,
		"wrong expression arity": `{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text","props":{"text":{"expr":{"op":"subtract","args":[1]}}}}}`,
	}
	for name, document := range invalidDocuments {
		t.Run(name, func(t *testing.T) {
			if err := validateNativeSchemaDocument(compiled, []byte(document)); err == nil {
				t.Fatal("validateNativeSchemaDocument() error = nil, want rejection")
			}
		})
	}
}

func TestGenerateValidatesTrustedNativeFixture(t *testing.T) {
	root := copyContractSources(t)
	fixture := `{"schemaVersion":1,"initialState":{},"root":{"id":"root","type":"Text","props":{"unknown":true}}}`
	if err := os.WriteFile(
		filepath.Join(root, "contracts", "card", "fixtures", "pomodoro-native.json"),
		[]byte(fixture),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(root); err == nil {
		t.Fatal("generate() error = nil, want invalid trusted fixture rejection")
	}
}

func TestGenerateRejectsInvalidCatalogSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "uniqueItems on numberList",
			mutate: func(catalog map[string]any) {
				values := catalog["components"].(map[string]any)["Chart"].(map[string]any)["allowedProps"].(map[string]any)["values"].(map[string]any)
				values["uniqueItems"] = true
			},
		},
		{
			name: "minimum above maximum",
			mutate: func(catalog map[string]any) {
				columns := catalog["components"].(map[string]any)["Grid"].(map[string]any)["allowedProps"].(map[string]any)["columns"].(map[string]any)
				columns["minimum"] = 7
				columns["maximum"] = 6
			},
		},
		{
			name: "maxArgs below minArgs",
			mutate: func(catalog map[string]any) {
				subtract := catalog["expressions"].(map[string]any)["subtract"].(map[string]any)
				subtract["maxArgs"] = 1
			},
		},
		{
			name: "invalid regex",
			mutate: func(catalog map[string]any) {
				valuePath := catalog["components"].(map[string]any)["TextInput"].(map[string]any)["allowedProps"].(map[string]any)["valuePath"].(map[string]any)
				valuePath["pattern"] = "["
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := copyContractSources(t)
			path := filepath.Join(root, "contracts", "card", "native-card-catalog.v1.json")
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var catalog map[string]any
			if err := json.Unmarshal(content, &catalog); err != nil {
				t.Fatal(err)
			}
			test.mutate(catalog)
			content, err = json.Marshal(catalog)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := generate(root); err == nil {
				t.Fatal("generate() error = nil, want catalog semantic rejection")
			}
		})
	}
}

func contractSources(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	read := func(parts ...string) []byte {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	return read("contracts", "card", "native-card-catalog.v1.json"),
		read("contracts", "card", "native-card.schema.json"),
		read("contracts", "local-rpc", "contract.json")
}

func schemaDefinition(document map[string]any, name string) map[string]any {
	return document["$defs"].(map[string]any)[name].(map[string]any)
}

func copyContractSources(t *testing.T) string {
	t.Helper()
	sourceRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, relative := range []string{
		"contracts/card/native-card-catalog.schema.json",
		"contracts/card/native-card-catalog.v1.json",
		"contracts/card/native-card.schema.json",
		"contracts/card/fixtures/pomodoro-native.json",
		"contracts/local-rpc/contract.json",
	} {
		content, err := os.ReadFile(filepath.Join(sourceRoot, relative))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
