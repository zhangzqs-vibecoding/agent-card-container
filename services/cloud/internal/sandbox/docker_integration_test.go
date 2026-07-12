package sandbox_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
)

func TestDockerBuilderRunsLockedDownContainer(t *testing.T) {
	image := os.Getenv("AGENTCARD_SANDBOX_TEST_IMAGE")
	if image == "" {
		t.Skip("AGENTCARD_SANDBOX_TEST_IMAGE is not configured")
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repository, "tooling", "codecard-template")
	workspace := t.TempDir()
	if err := copyTemplate(source, workspace); err != nil {
		t.Fatal(err)
	}

	builder := sandbox.NewDockerBuilder(sandbox.DockerConfig{
		Image:   image,
		Timeout: 5 * time.Minute,
	}, sandbox.ExecRunner{})
	output, err := builder.Build(
		context.Background(),
		sandbox.BuildRequest{Workspace: workspace},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"index.html",
		"dependency-policy.json",
	} {
		if len(output.Files[required]) == 0 {
			t.Fatalf("sandbox output is missing %s", required)
		}
	}
}

func copyTemplate(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == "node_modules" || info.Name() == "dist") {
			return filepath.SkipDir
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
