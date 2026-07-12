package agent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
)

type SandboxBuilder interface {
	Build(context.Context, sandbox.BuildRequest) (sandbox.BuildOutput, error)
}

type TemplateWebBuilder struct {
	template string
	sandbox  SandboxBuilder
}

func NewTemplateWebBuilder(template string, builder SandboxBuilder) *TemplateWebBuilder {
	return &TemplateWebBuilder{template: template, sandbox: builder}
}

func (builder *TemplateWebBuilder) Build(
	ctx context.Context,
	files map[string]string,
) (map[string][]byte, error) {
	workspace, err := os.MkdirTemp("", "agent-card-web-*")
	if err != nil {
		return nil, fmt.Errorf("create CodeCard workspace: %w", err)
	}
	defer os.RemoveAll(workspace)
	if err := copyTemplate(builder.template, workspace); err != nil {
		return nil, err
	}
	for name, source := range files {
		if name != "src/card.tsx" &&
			name != "src/card.css" &&
			name != "src/card.test.tsx" {
			return nil, fmt.Errorf("generated source path %q is not allowed", name)
		}
		target := filepath.Join(workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, fmt.Errorf("create generated source directory: %w", err)
		}
		if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
			return nil, fmt.Errorf("write generated source: %w", err)
		}
	}
	output, err := builder.sandbox.Build(ctx, sandbox.BuildRequest{
		Workspace: workspace,
	})
	if err != nil {
		return nil, err
	}
	filesOutput := make(map[string][]byte, len(output.Files))
	for name, content := range output.Files {
		filesOutput[name] = append([]byte(nil), content...)
	}
	return filesOutput, nil
}

func copyTemplate(sourceRoot, destinationRoot string) error {
	return filepath.WalkDir(sourceRoot, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("CodeCard template contains symbolic link")
		}
		relative, err := filepath.Rel(sourceRoot, source)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(destinationRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("CodeCard template contains non-regular file")
		}
		content, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o600)
	})
}
