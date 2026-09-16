package caddy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	err := deployOS(context.Background(), logger, NewTarget("caddy", map[string]string{"path": targetPath}), content)
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
	err := deployOS(context.Background(), logger, NewTarget("caddy", map[string]string{"path": targetPath}), content)
	require.Error(t, err)
	require.ErrorContains(t, err, "caddy validate failed")

	// Target file should not be created.
	_, err = os.Stat(targetPath)
	require.True(t, os.IsNotExist(err))

	// Staging file should be cleaned up.
	staging := filepath.Join(tmpDir, ".Caddyfile.tmp")
	_, err = os.Stat(staging)
	require.True(t, os.IsNotExist(err))
}

func TestDeployOSReloadFailureDaemonOffline(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "sub", "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "validate" {
			return exec.CommandContext(ctx, "sh", "-c", "exit 0")
		}

		return exec.CommandContext(ctx, "sh", "-c", "echo 'connection refused' >&2; exit 1")
	}

	content := []byte(":80 {\n\trespond \"offline\"\n}\n")
	err := deployOS(context.Background(), logger, NewTarget("edge", map[string]string{"path": targetPath}), content)
	require.NoError(t, err)

	// File should be written for next boot even if daemon wasn't running.
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

	err = deployOS(context.Background(), logger, NewTarget("caddy", map[string]string{"path": targetPath}), []byte("test"))
	require.Error(t, err)
	require.ErrorContains(t, err, "creating directory")
}
