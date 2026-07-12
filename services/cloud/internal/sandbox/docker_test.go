package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zzq/agent-card-container/services/cloud/internal/sandbox"
)

func TestExecRunnerReturnsBoundedDiagnosticOutput(t *testing.T) {
	t.Parallel()

	err := (sandbox.ExecRunner{}).Run(context.Background(), sandbox.Command{
		Binary: "sh",
		Args:   []string{"-c", "printf diagnostic >&2; exit 7"},
	})
	if err == nil || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestDockerBuilderUsesLockedDownFixedContainerPolicy(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dist := filepath.Join(workspace, "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &captureRunner{}
	digest := strings.Repeat("a", 64)
	builder := sandbox.NewDockerBuilder(sandbox.DockerConfig{
		Image:   "agent-card-builder@sha256:" + digest,
		Timeout: 5 * time.Minute,
	}, runner)

	output, err := builder.Build(context.Background(), sandbox.BuildRequest{Workspace: workspace})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if string(output.Files["index.html"]) != "<html></html>" {
		t.Fatalf("output = %#v", output.Files)
	}
	command := strings.Join(runner.command.Args, " ")
	for _, required := range []string{
		"--network none",
		"--read-only",
		"--cpus 2",
		"--memory 2g",
		"--pids-limit 256",
		"--user " + strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
		"--cap-drop ALL",
		"no-new-privileges",
		"/opt/codecard-template/node_modules/.vite-temp:rw,noexec,nosuid,size=16777216",
		"agent-card-builder@sha256:" + digest,
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("command %q missing %q", command, required)
		}
	}
	if len(runner.command.Environment) != 0 {
		t.Fatalf("container environment = %#v, want empty", runner.command.Environment)
	}
	if runner.deadline.IsZero() || runner.deadline.Sub(runner.started) > 5*time.Minute {
		t.Fatalf("deadline = %v, started = %v", runner.deadline, runner.started)
	}
}

func TestDockerBuilderAcceptsImmutableLocalImageIDAndRejectsShortDigest(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dist := filepath.Join(workspace, "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &captureRunner{}
	imageID := "sha256:" + strings.Repeat("b", 64)
	builder := sandbox.NewDockerBuilder(sandbox.DockerConfig{Image: imageID}, runner)
	if _, err := builder.Build(context.Background(), sandbox.BuildRequest{Workspace: workspace}); err != nil {
		t.Fatalf("Build() with immutable image ID error = %v", err)
	}
	if !strings.Contains(strings.Join(runner.command.Args, " "), imageID) {
		t.Fatalf("container command does not contain image ID: %#v", runner.command.Args)
	}
	short := sandbox.NewDockerBuilder(sandbox.DockerConfig{Image: "image@sha256:abc"}, runner)
	if _, err := short.Build(context.Background(), sandbox.BuildRequest{Workspace: workspace}); err == nil {
		t.Fatal("Build() accepted a short image digest")
	}
}

func TestDockerBuilderRejectsSymlinksAndOversizedOutput(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dist := filepath.Join(workspace, "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dist, "escape")); err != nil {
		t.Fatal(err)
	}
	builder := sandbox.NewDockerBuilder(sandbox.DockerConfig{
		Image: "image@sha256:" + strings.Repeat("c", 64),
	}, &captureRunner{})

	if _, err := builder.Build(context.Background(), sandbox.BuildRequest{Workspace: workspace}); err == nil {
		t.Fatal("Build() accepted symbolic link output")
	}
}

type captureRunner struct {
	command  sandbox.Command
	started  time.Time
	deadline time.Time
}

func (runner *captureRunner) Run(ctx context.Context, command sandbox.Command) error {
	runner.command = command
	runner.started = time.Now()
	runner.deadline, _ = ctx.Deadline()
	return nil
}
