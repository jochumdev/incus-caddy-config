package caddy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var execCommand = exec.CommandContext

// deployOS stages, validates, atomically renames, and reloads the Caddyfile on the local filesystem.
func deployOS(ctx context.Context, logger *slog.Logger, target OSTarget, content []byte) error {
	caddyfilePath := target.Path
	dir := filepath.Dir(caddyfilePath)

	err := os.MkdirAll(dir, 0o750)
	if err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	stagingPath := filepath.Join(dir, "."+filepath.Base(caddyfilePath)+".tmp")

	err = os.WriteFile(stagingPath, content, 0o600)
	if err != nil {
		return fmt.Errorf("writing staging file %s: %w", stagingPath, err)
	}

	validateCmd := execCommand(ctx, "caddy", "validate", "--config", stagingPath, "--adapter", "caddyfile")
	var validateOut bytes.Buffer
	validateCmd.Stdout = &validateOut
	validateCmd.Stderr = &validateOut

	err = validateCmd.Run()
	if err != nil {
		_ = os.Remove(stagingPath)

		return fmt.Errorf("caddy validate failed: %w (output: %q)", err, validateOut.String())
	}

	err = os.Rename(stagingPath, caddyfilePath)
	if err != nil {
		_ = os.Remove(stagingPath)

		return fmt.Errorf("atomic rename %s -> %s: %w", stagingPath, caddyfilePath, err)
	}

	reloadCmd := execCommand(ctx, "caddy", "reload", "--config", caddyfilePath, "--adapter", "caddyfile")
	var reloadOut bytes.Buffer
	reloadCmd.Stdout = &reloadOut
	reloadCmd.Stderr = &reloadOut

	reloadErr := reloadCmd.Run()
	if reloadErr != nil {
		logger.Info("Caddyfile written to disk; caddy reload not completed (daemon may not be running)",
			"label", target.Label,
			"path", caddyfilePath,
			"detail", strings.TrimSpace(reloadOut.String()),
		)

		//nolint:nilerr // Intentional: Caddy daemon may not be running yet; file is staged for boot.
		return nil
	}

	logger.Info("Caddyfile deployed and reloaded successfully on host",
		"label", target.Label,
		"path", caddyfilePath,
	)

	return nil
}
