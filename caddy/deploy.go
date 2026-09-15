package caddy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/pkg/sftp"

	"github.com/lxc/incus-compose/iclient"
)

type volumeTarget struct {
	pool      string
	name      string
	mountPath string
}

func resolveVolume(devices map[string]map[string]string, caddyfilePath string) *volumeTarget {
	if len(devices) == 0 {
		return nil
	}

	dir := filepath.Dir(caddyfilePath)

	for _, dev := range devices {
		if dev["type"] != "disk" {
			continue
		}

		mountPath := dev["path"]
		pool := dev["pool"]
		source := dev["source"]

		if mountPath == "" || pool == "" || source == "" {
			continue
		}

		if mountPath == dir || mountPath == caddyfilePath || strings.HasPrefix(caddyfilePath, strings.TrimSuffix(mountPath, "/")+"/") {
			return &volumeTarget{
				pool:      pool,
				name:      source,
				mountPath: mountPath,
			}
		}
	}

	return nil
}

// deploy stages, validates, atomically renames, and reloads the Caddyfile for a target.
func deploy(ctx context.Context, logger *slog.Logger, conn *iclient.Connection, target Target, caddyfilePath string, content []byte) error {
	if caddyfilePath == "" {
		caddyfilePath = "/config/Caddyfile"
	}

	inst, _, err := conn.GetInstance(ctx, target.Project, target.Instance, nil)
	if err != nil {
		return fmt.Errorf("getting instance %s:%s: %w", target.Project, target.Instance, err)
	}

	devices := inst.ExpandedDevices
	if len(devices) == 0 {
		devices = inst.Devices
	}

	vol := resolveVolume(devices, caddyfilePath)

	var sftpClient *sftp.Client
	var sftpStagingPath, sftpTargetPath string

	containerStagingPath := filepath.Join(
		filepath.Dir(caddyfilePath),
		"."+filepath.Base(caddyfilePath)+".tmp",
	)

	if vol != nil {
		relPath, err := filepath.Rel(vol.mountPath, caddyfilePath)
		if err != nil {
			relPath = filepath.Base(caddyfilePath)
		}

		sftpTargetPath = "/" + strings.TrimPrefix(relPath, "/")
		sftpStagingPath = "/" + strings.TrimPrefix(filepath.Join(filepath.Dir(relPath), "."+filepath.Base(relPath)+".tmp"), "/")

		logger.Debug("resolved storage volume for caddy",
			"pool", vol.pool,
			"volume", vol.name,
			"mount", vol.mountPath,
			"target", sftpTargetPath,
		)

		sftpClient, err = conn.GetStoragePoolVolumeFileSFTP(ctx, target.Project, vol.pool, "custom", vol.name)
		if err != nil {
			return fmt.Errorf("opening SFTP session for volume %s/%s: %w", vol.pool, vol.name, err)
		}
	} else {
		sftpTargetPath = caddyfilePath
		sftpStagingPath = containerStagingPath

		sftpClient, err = conn.GetInstanceFileSFTP(ctx, target.Project, target.Instance)
		if err != nil {
			return fmt.Errorf("opening SFTP session for %s:%s: %w", target.Project, target.Instance, err)
		}
	}

	defer func() {
		_ = sftpClient.Close()
	}()

	f, err := sftpClient.Create(sftpStagingPath)
	if err != nil {
		return fmt.Errorf("creating staging file %s: %w", sftpStagingPath, err)
	}

	_, err = f.Write(content)
	closeErr := f.Close()
	if err != nil {
		_ = sftpClient.Remove(sftpStagingPath)

		return fmt.Errorf("writing staging content: %w", err)
	}

	if closeErr != nil {
		_ = sftpClient.Remove(sftpStagingPath)

		return fmt.Errorf("closing staging file: %w", closeErr)
	}

	running := inst.StatusCode == incusapi.Running || inst.Status == "Running"
	if running {
		// Validate in-container using the real Caddy binary.
		var validateStdout, validateStderr bytes.Buffer
		validatePost := incusapi.InstanceExecPost{
			Command: []string{"caddy", "validate", "--config", containerStagingPath, "--adapter", "caddyfile"},
		}

		updates, err := conn.ExecInstance(ctx, target.Project, target.Instance, validatePost, &iclient.InstanceExecArgs{
			Stdout: &validateStdout,
			Stderr: &validateStderr,
		})
		if err != nil {
			_ = sftpClient.Remove(sftpStagingPath)

			return fmt.Errorf("executing caddy validate in %s:%s: %w", target.Project, target.Instance, err)
		}

		op, err := iclient.WaitOperation(ctx, updates)
		if err != nil {
			_ = sftpClient.Remove(sftpStagingPath)

			return fmt.Errorf("waiting for caddy validate: %w", err)
		}

		exitCode, ok := op.Metadata["return"].(float64)
		if !ok || int(exitCode) != 0 {
			_ = sftpClient.Remove(sftpStagingPath)

			return fmt.Errorf("caddy validate failed (exit %d): stdout: %q, stderr: %q",
				int(exitCode), validateStdout.String(), validateStderr.String())
		}
	}

	// Atomic rename over target path on the volume or instance.
	err = sftpClient.PosixRename(sftpStagingPath, sftpTargetPath)
	if err != nil {
		_ = sftpClient.Remove(sftpStagingPath)

		return fmt.Errorf("atomic rename %s -> %s: %w", sftpStagingPath, sftpTargetPath, err)
	}

	if !running {
		logger.Info("Caddyfile deployed to volume for next boot (instance not running)",
			"label", target.Label,
			"project", target.Project,
			"instance", target.Instance,
		)

		return nil
	}

	// Reload Caddy directly inside container.
	var reloadStdout, reloadStderr bytes.Buffer
	reloadPost := incusapi.InstanceExecPost{
		Command: []string{"caddy", "reload", "--config", caddyfilePath, "--adapter", "caddyfile"},
	}

	reloadUpdates, err := conn.ExecInstance(ctx, target.Project, target.Instance, reloadPost, &iclient.InstanceExecArgs{
		Stdout: &reloadStdout,
		Stderr: &reloadStderr,
	})
	if err != nil {
		return fmt.Errorf("executing caddy reload in %s:%s: %w", target.Project, target.Instance, err)
	}

	reloadOp, err := iclient.WaitOperation(ctx, reloadUpdates)
	if err != nil {
		return fmt.Errorf("waiting for caddy reload: %w", err)
	}

	reloadExitCode, ok := reloadOp.Metadata["return"].(float64)
	if !ok || int(reloadExitCode) != 0 {
		return fmt.Errorf("caddy reload failed (exit %d): stdout: %q, stderr: %q",
			int(reloadExitCode), reloadStdout.String(), reloadStderr.String())
	}

	logger.Info("Caddyfile deployed and reloaded successfully",
		"label", target.Label,
		"project", target.Project,
		"instance", target.Instance,
	)

	return nil
}
