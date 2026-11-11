package templated

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go/v5"
	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/pkg/sftp"

	"github.com/lxc/incus-compose/iclient"
)

type volumeTarget struct {
	pool      string
	name      string
	mountPath string
}

func resolveVolume(devices map[string]map[string]string, path string) *volumeTarget {
	if len(devices) == 0 {
		return nil
	}

	dir := filepath.Dir(path)

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

		if mountPath == dir || mountPath == path || strings.HasPrefix(path, strings.TrimSuffix(mountPath, "/")+"/") {
			return &volumeTarget{
				pool:      pool,
				name:      source,
				mountPath: mountPath,
			}
		}
	}

	return nil
}

// expand substitutes {staging} and {path} in each argument of a command.
func expand(argv []string, staging, path string) []string {
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		a = strings.ReplaceAll(a, "{staging}", staging)
		out = append(out, strings.ReplaceAll(a, "{path}", path))
	}

	return out
}

func execIn(ctx context.Context, conn *iclient.Connection, project, instance string, argv []string) error {
	var stdout, stderr bytes.Buffer

	updates, err := conn.ExecInstance(ctx, project, instance, incusapi.InstanceExecPost{Command: argv}, &iclient.InstanceExecArgs{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("executing %q in %s:%s: %w", argv, project, instance, err)
	}

	op, err := iclient.WaitOperation(ctx, updates)
	if err != nil {
		return fmt.Errorf("waiting for %q: %w", argv, err)
	}

	exitCode, ok := op.Metadata["return"].(float64)
	if !ok || int(exitCode) != 0 {
		return fmt.Errorf("%q failed (exit %d): stdout: %q, stderr: %q", argv, int(exitCode), stdout.String(), stderr.String())
	}

	return nil
}

// deploy stages, checks, atomically renames, and reloads the rendered file inside an instance.
func deploy(ctx context.Context, logger *slog.Logger, conn *iclient.Connection, target Target, content []byte) error {
	return retry.New(
		retry.Context(ctx),
		retry.Attempts(10),
		retry.Delay(250*time.Millisecond),
		retry.MaxDelay(3*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			logger.Warn("retrying deployment", "attempt", n+1, "target", target.String(), "err", err)
		}),
	).Do(func() error {
		inst, _, err := conn.GetInstance(ctx, target.Project, target.Instance, nil)
		if err != nil {
			return fmt.Errorf("getting instance %s:%s: %w", target.Project, target.Instance, err)
		}

		devices := inst.ExpandedDevices
		if len(devices) == 0 {
			devices = inst.Devices
		}

		vol := resolveVolume(devices, target.Path)

		var sftpClient *sftp.Client
		var sftpStagingPath, sftpTargetPath string

		staging := filepath.Join(filepath.Dir(target.Path), filepath.Base(target.Path)+".tmp")

		if vol != nil {
			relPath, err := filepath.Rel(vol.mountPath, target.Path)
			if err != nil {
				relPath = filepath.Base(target.Path)
			}

			sftpTargetPath = "/" + strings.TrimPrefix(relPath, "/")
			sftpStagingPath = "/" + strings.TrimPrefix(filepath.Join(filepath.Dir(relPath), filepath.Base(relPath)+".tmp"), "/")

			logger.Debug("resolved storage volume",
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
			sftpTargetPath = target.Path
			sftpStagingPath = staging

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
			return fmt.Errorf("writing staging content: %w", err)
		}

		if closeErr != nil {
			return fmt.Errorf("closing staging file: %w", closeErr)
		}

		// A stopped instance cannot exec; the file is still swapped in so it boots with it.
		running := inst.StatusCode == incusapi.Running || inst.Status == "Running"
		if running {
			for _, argv := range [][]string{target.Format, target.Validate} {
				if len(argv) == 0 {
					continue
				}

				err := execIn(ctx, conn, target.Project, target.Instance, expand(argv, staging, target.Path))
				if err != nil {
					return err
				}
			}
		}

		err = sftpClient.PosixRename(sftpStagingPath, sftpTargetPath)
		if err != nil {
			return fmt.Errorf("atomic rename %s -> %s: %w", sftpStagingPath, sftpTargetPath, err)
		}

		if !running {
			logger.Info("Deployed for next boot (instance not running)", "target", target.String())

			return nil
		}

		if len(target.Reload) > 0 {
			err := execIn(ctx, conn, target.Project, target.Instance, expand(target.Reload, staging, target.Path))
			if err != nil {
				return err
			}
		}

		logger.Info("Deployed", "target", target.String())

		return nil
	})
}
