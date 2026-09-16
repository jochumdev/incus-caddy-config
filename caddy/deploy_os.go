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
	"time"

	"github.com/avast/retry-go/v5"
)

var execCommand = exec.CommandContext

// deployOS stages, validates, atomically renames, and reloads the Caddyfile on the local filesystem.
func deployOS(ctx context.Context, logger *slog.Logger, target Target, content []byte) error {
	caddyfilePath, _ := target.Flag("path")

	return retry.New(
		retry.Context(ctx),
		retry.Attempts(10),
		retry.Delay(250*time.Millisecond),
		retry.MaxDelay(3*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Warn("retrying OS Caddyfile deployment",
				"attempt", n+1,
				"label", target.Label,
				"path", caddyfilePath,
				"err", err,
			)
		}),
	).Do(func() error {
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

		fmtCmd := execCommand(ctx, "caddy", "fmt", "--overwrite", stagingPath)
		var fmtOut bytes.Buffer
		fmtCmd.Stdout = &fmtOut
		fmtCmd.Stderr = &fmtOut

		err = fmtCmd.Run()
		if err != nil {
			return fmt.Errorf("caddy fmt failed: %w (output: %q)", err, fmtOut.String())
		}

		err = os.Rename(stagingPath, caddyfilePath)
		if err != nil {
			return fmt.Errorf("atomic rename %s -> %s: %w", stagingPath, caddyfilePath, err)
		}

		reloadCmd := execCommand(ctx, "caddy", "reload", "--config", caddyfilePath, "--adapter", "caddyfile")
		var reloadOut bytes.Buffer
		reloadCmd.Stdout = &reloadOut
		reloadCmd.Stderr = &reloadOut

		reloadErr := reloadCmd.Run()
		if reloadErr != nil {
			outStr := reloadOut.String()
			if strings.Contains(outStr, "connection refused") || strings.Contains(outStr, "no such file or directory") {
				logger.Info("Caddyfile written to disk; caddy reload not completed (daemon may not be running)",
					"label", target.Label,
					"path", caddyfilePath,
					"detail", strings.TrimSpace(outStr),
				)

				return nil
			}

			return fmt.Errorf("caddy reload failed: %w (output: %q)", reloadErr, outStr)
		}

		logger.Info("Caddyfile deployed and reloaded successfully on host",
			"label", target.Label,
			"path", caddyfilePath,
		)

		return nil
	})
}
