package templated

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func caddyLike(path string) Target {
	return Target{
		Label:    "caddy",
		Path:     path,
		Format:   []string{"caddy", "fmt", "--overwrite", "{staging}"},
		Validate: []string{"caddy", "validate", "--config", "{staging}"},
		Reload:   []string{"caddy", "reload", "--config", "{path}"},
	}
}

func TestDeployOSWithoutCommands(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "sd", "targets.json")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(_ context.Context, name string, _ ...string) *exec.Cmd {
		t.Fatalf("no command configured, but %q ran", name)

		return nil
	}

	err := deployOS(context.Background(), logger, Target{Label: "prom", Path: targetPath}, []byte("[]"))
	require.NoError(t, err)

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, "[]", string(data))
}

func TestDeployOSExpandsPlaceholders(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	var ran [][]string

	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		ran = append(ran, append([]string{name}, args...))

		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	err := deployOS(context.Background(), logger, caddyLike(targetPath), []byte("x"))
	require.NoError(t, err)

	staging := targetPath + ".tmp"
	require.Equal(t, [][]string{
		{"caddy", "fmt", "--overwrite", staging},
		{"caddy", "validate", "--config", staging},
		{"caddy", "reload", "--config", targetPath},
	}, ran)
}

func TestDeployOSSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	content := []byte(":80 {\n\trespond \"ok\"\n}\n")
	err := deployOS(context.Background(), logger, caddyLike(targetPath), content)
	require.NoError(t, err)

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, content, data)

	staging := filepath.Join(tmpDir, ".Caddyfile.tmp")
	_, err = os.Stat(staging)
	require.True(t, os.IsNotExist(err))
}

func TestDeployOSValidateFailure(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo 'invalid syntax' >&2; exit 1")
	}

	content := []byte("invalid content")
	err := deployOS(context.Background(), logger, caddyLike(targetPath), content)
	require.Error(t, err)
	require.ErrorContains(t, err, `["caddy" "fmt" "--overwrite"`)

	// Target file should not be created.
	_, err = os.Stat(targetPath)
	require.True(t, os.IsNotExist(err))

	// Staging file should be preserved on failure for inspection.
	staging := filepath.Join(tmpDir, "Caddyfile.tmp")
	_, err = os.Stat(staging)
	require.NoError(t, err)
}

func TestDeployOSReloadFailureIsAnError(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "sub", "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) > 0 && (args[0] == "fmt" || args[0] == "validate") {
			return exec.CommandContext(ctx, "sh", "-c", "exit 0")
		}

		return exec.CommandContext(ctx, "sh", "-c", "echo 'connection refused' >&2; exit 1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	content := []byte(":80 {\n\trespond \"offline\"\n}\n")
	err := deployOS(ctx, logger, caddyLike(targetPath), content)
	require.Error(t, err)

	// The file is in place for the next start even though the reload failed.
	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, content, data)
}

func TestDeployOSDirectoryCreationFailure(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "not-a-dir")
	err := os.WriteFile(tmpFile, []byte("file"), 0o600)
	require.NoError(t, err)

	targetPath := filepath.Join(tmpFile, "cannot-create", "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	err = deployOS(context.Background(), logger, caddyLike(targetPath), []byte("test"))
	require.Error(t, err)
	require.ErrorContains(t, err, "creating directory")
}

func TestDeployOSRetryCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo 'transient lock failure' >&2; exit 1")
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	content := []byte(":80 {\n\trespond \"test\"\n}\n")
	err := deployOS(ctx, logger, caddyLike(targetPath), content)
	require.Error(t, err)
	require.Less(t, time.Since(start), 2*time.Second)
}
