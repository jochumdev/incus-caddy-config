package caddy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"sync"

	"github.com/lxc/incus-compose/iclient"
	"github.com/lxc/incus-compose/ievent/iutil"
)

type deployResult struct {
	targetKey string
	hash      []byte
}

type deployState struct {
	lastDeployed map[string][]byte
	cancels      map[string]context.CancelFunc
	done         chan deployResult
	wg           sync.WaitGroup
}

func newDeployState() *deployState {
	return &deployState{
		lastDeployed: make(map[string][]byte),
		cancels:      make(map[string]context.CancelFunc),
		done:         make(chan deployResult, 32),
	}
}

func cancelAll(cancels map[string]context.CancelFunc) {
	for _, cancel := range cancels {
		cancel()
	}
}

func reconcileDeployments(
	ctx context.Context,
	state *deployState,
	logger *slog.Logger,
	conn *iclient.Connection,
	cfg Config,
	instances []*iutil.Event,
) {
	for _, target := range cfg.Targets {
		project, _ := target.Flag("project")
		instance, _ := target.Flag("instance")
		targetKey := project + "/" + instance

		vhosts := extractVhosts(target.Label, instances)

		globalTmpl, _ := target.Flag("global_template")
		if globalTmpl == "" {
			globalTmpl, _ = target.Flag("global-template")
		}

		content, err := render(vhosts, cfg.TemplatesDir, globalTmpl)
		if err != nil {
			logger.Error("rendering Caddyfile", "label", target.Label, "err", err)

			continue
		}

		sum := sha256.Sum256(content)
		last := state.lastDeployed[targetKey]
		if bytes.Equal(last, sum[:]) {
			continue
		}

		if conn == nil {
			continue
		}

		cancel, ok := state.cancels[targetKey]
		if ok {
			cancel()
		}

		deployCtx, cancel := context.WithCancel(ctx)
		state.cancels[targetKey] = cancel

		state.wg.Go(func() {
			err := deploy(deployCtx, logger, conn, target, cfg.CaddyfilePath, content)
			if err != nil {
				logger.Error("deploying Caddyfile", "target", targetKey, "label", target.Label, "err", err)

				return
			}

			select {
			case state.done <- deployResult{targetKey: targetKey, hash: bytes.Clone(sum[:])}:
			case <-deployCtx.Done():
			}
		})
	}

	for _, target := range cfg.OSTargets {
		path, _ := target.Flag("path")
		targetKey := "os:" + target.Label + ":" + path

		vhosts := extractVhosts(target.Label, instances)

		globalTmpl, _ := target.Flag("global_template")
		if globalTmpl == "" {
			globalTmpl, _ = target.Flag("global-template")
		}

		content, err := render(vhosts, cfg.TemplatesDir, globalTmpl)
		if err != nil {
			logger.Error("rendering Caddyfile for OS target", "label", target.Label, "target", target.String(), "err", err)

			continue
		}

		sum := sha256.Sum256(content)
		last := state.lastDeployed[targetKey]
		if bytes.Equal(last, sum[:]) {
			continue
		}

		cancel, ok := state.cancels[targetKey]
		if ok {
			cancel()
		}

		deployCtx, cancel := context.WithCancel(ctx)
		state.cancels[targetKey] = cancel

		state.wg.Go(func() {
			err := deployOS(deployCtx, logger, target, content)
			if err != nil {
				logger.Error("deploying Caddyfile for OS target", "label", target.Label, "target", target.String(), "err", err)

				return
			}

			select {
			case state.done <- deployResult{targetKey: targetKey, hash: bytes.Clone(sum[:])}:
			case <-deployCtx.Done():
			}
		})
	}
}
