package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Command struct {
	Binary      string
	Args        []string
	Environment []string
	Directory   string
}

type CommandRunner interface {
	Run(context.Context, Command) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Binary, command.Args...)
	process.Dir = command.Directory
	if len(command.Environment) > 0 {
		process.Env = append(os.Environ(), command.Environment...)
	}
	if err := process.Run(); err != nil {
		return fmt.Errorf("sandbox process failed: %w", err)
	}
	return nil
}

type DockerConfig struct {
	Binary  string
	Image   string
	Timeout time.Duration
}

type BuildRequest struct {
	Workspace string
}

type BuildOutput struct {
	Files map[string][]byte
}

type DockerBuilder struct {
	config DockerConfig
	runner CommandRunner
}

func NewDockerBuilder(config DockerConfig, runner CommandRunner) *DockerBuilder {
	if config.Binary == "" {
		config.Binary = "docker"
	}
	if config.Timeout <= 0 || config.Timeout > 5*time.Minute {
		config.Timeout = 5 * time.Minute
	}
	return &DockerBuilder{config: config, runner: runner}
}

func (builder *DockerBuilder) Build(ctx context.Context, request BuildRequest) (BuildOutput, error) {
	workspace, err := filepath.Abs(request.Workspace)
	if err != nil {
		return BuildOutput{}, fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return BuildOutput{}, fmt.Errorf("sandbox workspace is unavailable")
	}
	if !strings.Contains(builder.config.Image, "@sha256:") {
		return BuildOutput{}, fmt.Errorf("sandbox image must be pinned by digest")
	}
	runContext, cancel := context.WithTimeout(ctx, builder.config.Timeout)
	defer cancel()
	command := Command{
		Binary: builder.config.Binary,
		Args: []string{
			"run", "--rm",
			"--network", "none",
			"--read-only",
			"--cpus", "2",
			"--memory", "2g",
			"--pids-limit", "256",
			"--user", "65532:65532",
			"--cap-drop", "ALL",
			"--security-opt", "no-new-privileges",
			"--tmpfs", "/tmp:rw,noexec,nosuid,size=268435456",
			"--mount", "type=bind,src=" + workspace + ",dst=/workspace,rw",
			"--workdir", "/workspace",
			builder.config.Image,
			"/bin/sh", "-lc",
			"ln -s /opt/codecard-template/node_modules /workspace/node_modules && pnpm run typecheck && pnpm test -- --run && pnpm run build && pnpm run validate:bundle",
		},
		Directory: workspace,
	}
	if err := builder.runner.Run(runContext, command); err != nil {
		return BuildOutput{}, err
	}
	return collectOutput(filepath.Join(workspace, "dist"))
}

func collectOutput(root string) (BuildOutput, error) {
	files := make(map[string][]byte)
	var total int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("sandbox output contains symbolic link")
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sandbox output contains non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, "../") ||
			len(strings.Split(relative, "/")) > 8 {
			return fmt.Errorf("sandbox output path is invalid")
		}
		if len(files) >= 512 || info.Size() > 8*1024*1024 {
			return fmt.Errorf("sandbox output exceeds file limits")
		}
		total += info.Size()
		if total > 32*1024*1024 {
			return fmt.Errorf("sandbox output exceeds expanded size limit")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relative] = content
		return nil
	})
	if err != nil {
		return BuildOutput{}, fmt.Errorf("collect sandbox output: %w", err)
	}
	if _, exists := files["index.html"]; !exists {
		return BuildOutput{}, fmt.Errorf("sandbox output is missing index.html")
	}
	return BuildOutput{Files: files}, nil
}
