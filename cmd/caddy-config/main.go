// Command caddy-config reconciles Caddy reverse proxy configurations from Incus instance events.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"go.uber.org/automaxprocs/maxprocs"

	_ "github.com/KimMachineGun/automemlimit"

	"github.com/lxc/incus-compose/iclient"
	ievlog "github.com/lxc/incus-compose/ievent/log"
	"github.com/lxc/incus-compose/ievent/source"
	"github.com/lxc/incus-compose/incustrust"
	"github.com/lxc/incus-compose/shared"
)

const certName = "caddy-config"

const drainTimeout = 30 * time.Second

func main() {
	err := command().Run(context.Background(), os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func command() *cli.Command {
	return &cli.Command{
		Name:  "caddy-config",
		Usage: "Dynamic Caddy configuration from Incus events",
		Commands: []*cli.Command{
			runCommand(newConfig()),
			versionCommand(),
		},
	}
}

func runCommand(cfg *config) *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Reconcile Caddy configurations from Incus events",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "incus",
				Usage:       "URL of the Incus API",
				Destination: &cfg.IncusURL,
				Sources:     cli.EnvVars("INCUS_CADDY_INCUS"),
			},
			&cli.StringFlag{
				Name:        "token",
				Usage:       "One-time trust token; a token file under --secrets-dir is read when this is empty",
				Destination: &cfg.Token,
				Sources:     cli.EnvVars("INCUS_CADDY_TOKEN"),
			},
			&cli.StringFlag{
				Name:        "data-dir",
				Usage:       "Persistent directory holding the enrolled certificate; empty keeps none",
				Value:       defaultDataDir,
				Destination: &cfg.DataDir,
				Sources:     cli.EnvVars("INCUS_CADDY_DATA_DIR"),
			},
			&cli.StringFlag{
				Name:        "secrets-dir",
				Usage:       "Tmpfs directory holding the one-time trust token",
				Value:       defaultSecretsDir,
				Destination: &cfg.SecretsDir,
				Sources:     cli.EnvVars("INCUS_CADDY_SECRETS_DIR"),
			},
			&cli.StringFlag{
				Name:        "client-cert",
				Usage:       "Certificate to present instead of enrolling; needs --client-key",
				Destination: &cfg.ClientCert,
				Sources:     cli.EnvVars("INCUS_CADDY_CLIENT_CERT"),
			},
			&cli.StringFlag{
				Name:        "client-key",
				Usage:       "Key for --client-cert",
				Destination: &cfg.ClientKey,
				Sources:     cli.EnvVars("INCUS_CADDY_CLIENT_KEY"),
			},
			&cli.BoolFlag{
				Name:        "restricted",
				Usage:       "Enroll a certificate confined to --project",
				Destination: &cfg.Restricted,
				Sources:     cli.EnvVars("INCUS_CADDY_RESTRICTED"),
			},
			&cli.StringFlag{
				Name:        "remote",
				Usage:       "Connect as a remote from the Incus CLI configuration; needs --use-remote, empty means the default remote",
				Destination: &cfg.Remote,
				Sources:     cli.EnvVars("INCUS_REMOTE"),
			},
			&cli.BoolFlag{
				Name:        "use-remote",
				Usage:       "Allow the Incus CLI configuration to be used when there is no certificate and no token",
				Destination: &cfg.UseRemote,
				Sources:     cli.EnvVars("INCUS_CADDY_USE_REMOTE"),
			},
			&cli.StringSliceFlag{
				Name:        "project",
				Usage:       "Project(s) to monitor for routed instances; empty means every visible project",
				Destination: &cfg.Projects,
				Sources:     cli.EnvVars("INCUS_CADDY_PROJECTS"),
			},
			&cli.StringSliceFlag{
				Name:        "caddy-instance",
				Usage:       "Target Caddy server in format 'label:project:instance'; can be repeated",
				Destination: &cfg.CaddyInstances,
				Sources:     cli.EnvVars("INCUS_CADDY_INSTANCES"),
			},
			&cli.StringSliceFlag{
				Name:        "os-path",
				Usage:       "Target local Caddyfile in format '[label:]path'; can be repeated",
				Destination: &cfg.OSTargets,
				Sources:     cli.EnvVars("INCUS_CADDY_OS_PATH"),
			},
			&cli.StringFlag{
				Name:        "caddyfile-path",
				Usage:       "Path to Caddyfile inside Caddy container",
				Value:       defaultCaddyfilePath,
				Destination: &cfg.CaddyfilePath,
				Sources:     cli.EnvVars("INCUS_CADDY_CADDYFILE_PATH"),
			},
			&cli.StringFlag{
				Name:        "templates-dir",
				Usage:       "Directory containing custom vhost templates",
				Destination: &cfg.TemplatesDir,
				Sources:     cli.EnvVars("INCUS_CADDY_TEMPLATES_DIR"),
			},
			&cli.StringSliceFlag{
				Name:        "global-template",
				Usage:       "Global Caddyfile template in format 'label:path-or-template'; can be repeated",
				Destination: &cfg.GlobalTemplates,
				Sources:     cli.EnvVars("INCUS_CADDY_GLOBAL_TEMPLATE"),
			},
			&cli.DurationFlag{
				Name:        "debounce-window",
				Usage:       "How long a key must be quiet before the last of its burst is handed on",
				Value:       defaultDebounceWindow,
				Destination: &cfg.DebounceWindow,
				Sources:     cli.EnvVars("INCUS_CADDY_DEBOUNCE_WINDOW"),
			},
			&cli.StringFlag{
				Name:        "http-address",
				Usage:       "Address to serve /health and /ready on; empty disables it",
				Value:       defaultHTTPAddr,
				Destination: &cfg.HTTPAddr,
				Sources:     cli.EnvVars("INCUS_CADDY_HTTP_ADDRESS"),
			},
			&cli.StringSliceFlag{
				Name:        "exclude",
				Usage:       "Chain position(s) to leave out; only optional ones",
				Destination: &cfg.Exclude,
				Sources:     cli.EnvVars("INCUS_CADDY_EXCLUDE"),
			},
			&cli.StringFlag{
				Name:        "log",
				Usage:       "Log level for the chain and process: TRACE, DEBUG, INFO, WARN, ERROR",
				Destination: &cfg.Log,
				Sources:     cli.EnvVars("INCUS_CADDY_LOG"),
			},
			&cli.BoolFlag{
				Name:        "pprof",
				Usage:       "Serve /debug/pprof on the --http-address; for profiling only",
				Destination: &cfg.Pprof,
				Sources:     cli.EnvVars("INCUS_CADDY_PPROF"),
			},
			&cli.IntFlag{
				Name:        "workers",
				Usage:       "Incus reads in flight at once",
				Value:       defaultWorkers,
				Destination: &cfg.Workers,
				Sources:     cli.EnvVars("INCUS_CADDY_WORKERS"),
			},
			&cli.DurationFlag{
				Name:        "read-timeout",
				Usage:       "Budget for one read of the daemon",
				Value:       defaultReadTimeout,
				Destination: &cfg.ReadTimeout,
				Sources:     cli.EnvVars("INCUS_CADDY_READ_TIMEOUT"),
			},
			&cli.DurationFlag{
				Name:        "sweep-project-delay",
				Usage:       "Gap between one project of a round and the next",
				Value:       defaultProjectDelay,
				Destination: &cfg.ProjectDelay,
				Sources:     cli.EnvVars("INCUS_CADDY_SWEEP_PROJECT_DELAY"),
			},
			&cli.DurationFlag{
				Name:        "sweep-read-delay",
				Usage:       "Gap between the reads inside one project",
				Value:       defaultReadDelay,
				Destination: &cfg.ReadDelay,
				Sources:     cli.EnvVars("INCUS_CADDY_SWEEP_READ_DELAY"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args, err := cfg.validate()
			if err != nil {
				return err
			}

			return mainAction(ctx, args)
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print version information",
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Println("caddy-config version", version)

			return nil
		},
	}
}

func mainAction(ctx context.Context, args *mainActionArgs) error {
	level := shared.StringToSlogLevel(args.Log)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ievlog.Hook(level)

	undo, err := maxprocs.Set(maxprocs.Logger(func(format string, a ...any) {
		logger.Info(fmt.Sprintf(format, a...))
	}))
	if err != nil {
		logger.Warn("setting GOMAXPROCS", "err", err)
	}

	defer undo()

	logger.Info("Starting",
		"version", version,
		"pid", os.Getpid(),
		"incus", args.endpoint(),
		"http", args.HTTPAddr,
		"caddyfile", args.CaddyfilePath,
	)

	logger.Debug("configuration",
		"projects", args.Projects,
		"targets", args.Targets,
		"os_targets", args.OSTargets,
		"data_dir", args.DataDir,
		"secrets_dir", args.SecretsDir,
		"token", args.redactedToken(),
		"templates_dir", args.TemplatesDir,
		"global_templates", args.GlobalTemplates,
		"debounce_window", args.DebounceWindow,
		"workers", args.Workers,
		"read_timeout", args.ReadTimeout,
		"sweep_project_delay", args.ProjectDelay,
		"sweep_read_delay", args.ReadDelay,
		"pprof", args.Pprof,
	)

	return run(ctx, logger, args)
}

func run(ctx context.Context, logger *slog.Logger, args *mainActionArgs) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	positions := chain(logger, args)
	plugins, runners, err := assemble(positions, args.Exclude)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.Name())
	}

	logger.Info("chain", "plugins", names)

	trust := incustrust.Config{
		Name:       certName,
		UserAgent:  certName + "/" + version,
		URL:        args.IncusURL,
		ClientCert: args.ClientCert,
		ClientKey:  args.ClientKey,
		Token:      args.Token,
		DataDir:    args.DataDir,
		SecretsDir: args.SecretsDir,
		Restricted: args.Restricted,
		Projects:   args.Projects,
		Remote:     args.Remote,
		UseRemote:  args.UseRemote,
	}

	var conn *iclient.Connection
	for {
		conn, err = incustrust.Connect(ctx, trust)
		if err == nil {
			logger.Info("Connected to Incus")

			break
		}

		if errors.Is(err, incustrust.ErrNoCredentials) {
			return err
		}

		logger.Error("connecting to Incus", "err", err)

		select {
		case <-ctx.Done():
			return fmt.Errorf("connecting to Incus: %w", err)
		case <-time.After(time.Second):
		}
	}

	src, err := source.New(logger, conn, plugins)
	if err != nil {
		return fmt.Errorf("building the source: %w", err)
	}

	sourceCtx, stopSource := context.WithCancel(ctx)
	defer stopSource()

	var srcWg, pluginWg sync.WaitGroup

	srcWg.Go(func() {
		err := src.Run(sourceCtx)
		if err != nil {
			logger.Error("running the source", "err", err)
			cancel()
		}
	})

	for _, r := range runners {
		pluginWg.Go(func() {
			err := r.Run(ctx)
			if err != nil {
				logger.Error("running a plugin", "plugin", r.Name(), "err", err)
				cancel()
			}

			src.Finished(r)
		})
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	defer signal.Stop(sig)

	select {
	case s := <-sig:
		logger.Info("shutting down", "signal", s.String())
	case <-ctx.Done():
	}

	stopSource()
	srcWg.Wait()

	src.Drain(drainContext(ctx))

	cancel()
	pluginWg.Wait()

	return nil
}

func drainContext(ctx context.Context) context.Context {
	out, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainTimeout)
	context.AfterFunc(out, cancel)

	return out
}
