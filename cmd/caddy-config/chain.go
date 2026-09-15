package main

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/lxc/incus-compose/ievent/debounce"
	"github.com/lxc/incus-compose/ievent/enricher"
	"github.com/lxc/incus-compose/ievent/http"
	"github.com/lxc/incus-compose/ievent/iutil"
	"github.com/lxc/incus-compose/ievent/log"
	"github.com/lxc/incus-compose/shared"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

type position struct {
	plugin   iutil.Plugin
	optional bool
}

type runner interface {
	iutil.Plugin
	Run(ctx context.Context) error
}

func chain(logger *slog.Logger, args *mainActionArgs) []position {
	out := []position{}

	trace := shared.StringToSlogLevel(args.Log) <= shared.LevelTrace

	if trace {
		out = append(out, logAt(logger, args.Log, "arrival")...)
	}

	out = append(out,
		position{plugin: debounce.New(logger, debounce.Window(args.DebounceWindow)), optional: true},
	)

	if trace {
		out = append(out, logAt(logger, args.Log, "received")...)
	}

	out = append(out,
		position{plugin: enricher.New(
			logger,
			enricher.Workers(args.Workers),
			enricher.ReadTimeout(args.ReadTimeout),
			enricher.ReadDelay(args.ReadDelay),
			enricher.Project(serves(logger, args.Projects)),
		)},
	)

	out = append(out, logAt(logger, args.Log, "enriched")...)

	out = append(out,
		position{plugin: caddy.New(
			logger,
			caddy.Config{
				Targets:            args.Targets,
				OSTargets:          args.OSTargets,
				CaddyfilePath:      args.CaddyfilePath,
				CustomTemplatesDir: args.CustomTemplatesDir,
				GlobalTemplate:     args.GlobalTemplate,
			},
		)},
		position{plugin: http.New(logger, http.Listen(args.HTTPAddr), http.Pprof(args.Pprof)), optional: true},
	)

	out = append(out, logAt(logger, args.Log, "served")...)

	return out
}

func logAt(logger *slog.Logger, logLevel string, at string) []position {
	if logLevel == "" {
		return nil
	}

	return []position{{plugin: log.New(logger, log.At(at), log.Level(logLevel)), optional: true}}
}

func assemble(positions []position, exclude []string) ([]iutil.Plugin, []runner, error) {
	optional := []string{}

	for _, p := range positions {
		if p.optional {
			optional = append(optional, p.plugin.Name())
		}
	}

	for _, name := range exclude {
		if slices.Contains(optional, name) {
			continue
		}

		return nil, nil, fmt.Errorf(
			"cannot exclude %q; this binary allows %s",
			name, strings.Join(optional, ", "),
		)
	}

	var (
		plugins []iutil.Plugin
		runners []runner
	)

	for _, p := range positions {
		if slices.Contains(exclude, p.plugin.Name()) {
			continue
		}

		plugins = append(plugins, p.plugin)

		r, ok := p.plugin.(runner)
		if ok {
			runners = append(runners, r)
		}
	}

	return plugins, runners, nil
}

func serves(logger *slog.Logger, projects []string) func(*incusapi.Project) bool {
	if len(projects) == 0 {
		return nil
	}

	return func(p *incusapi.Project) bool {
		serve := slices.Contains(projects, p.Name)
		if !serve {
			logger.Debug("Not serving project", "project", p.Name)
		} else {
			logger.Log(context.Background(), shared.LevelTrace, "Serving project", "project", p.Name)
		}

		return serve
	}
}
