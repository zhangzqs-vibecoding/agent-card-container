package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zzq/agent-card-container/services/cloud/internal/agent"
	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
)

func TestTemplateWebBuilderCopiesFixedTemplateAndGeneratedWhitelist(t *testing.T) {
	t.Parallel()

	template := t.TempDir()
	if err := os.MkdirAll(filepath.Join(template, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template, "package.json"), []byte(`{"private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(template, "src", "card.tsx"), []byte("template"), 0o600); err != nil {
		t.Fatal(err)
	}
	builder := &captureSandboxBuilder{}
	webBuilder := agent.NewTemplateWebBuilder(template, builder)

	output, err := webBuilder.Build(context.Background(), map[string]string{
		"src/card.tsx": "export function Card(){return <div>生成</div>}",
		"src/card.css": "div{color:white}",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if string(builder.cardSource) != "export function Card(){return <div>生成</div>}" {
		t.Fatalf("card source = %q", builder.cardSource)
	}
	if string(builder.packageSource) != `{"private":true}` {
		t.Fatalf("package source = %q", builder.packageSource)
	}
	if string(output["index.html"]) != "<html>built</html>" {
		t.Fatalf("output = %#v", output)
	}
	if _, err := os.Stat(builder.workspace); !os.IsNotExist(err) {
		t.Fatalf("temporary workspace still exists: %v", err)
	}
}

type captureSandboxBuilder struct {
	workspace     string
	cardSource    []byte
	packageSource []byte
}

func (builder *captureSandboxBuilder) Build(_ context.Context, request sandbox.BuildRequest) (sandbox.BuildOutput, error) {
	builder.workspace = request.Workspace
	var err error
	builder.cardSource, err = os.ReadFile(filepath.Join(request.Workspace, "src", "card.tsx"))
	if err != nil {
		return sandbox.BuildOutput{}, err
	}
	builder.packageSource, err = os.ReadFile(filepath.Join(request.Workspace, "package.json"))
	if err != nil {
		return sandbox.BuildOutput{}, err
	}
	return sandbox.BuildOutput{
		Files: map[string][]byte{"index.html": []byte("<html>built</html>")},
	}, nil
}
