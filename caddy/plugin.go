// Package caddy generates and deploys Caddy configurations from Incus instance labels.
package caddy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"strings"

	"github.com/lxc/incus-compose/iclient"
	"github.com/lxc/incus-compose/ievent/iutil"
)

const name = "caddy"

const defaultInboxSize = 256

// Config configures the Caddy event plugin.
type Config struct {
	Targets         []Target
	OSTargets       []Target
	CaddyfilePath   string
	TemplatesDir    string
	GlobalTemplates map[string]string
	InboxSize       int
}

// Plugin consumes enriched instance events and deploys Caddyfiles to target instances.
type Plugin struct {
	logger *slog.Logger
	cfg    Config
	conn   *iclient.Connection

	next       iutil.Next
	commandIn  <-chan iutil.Command
	commandOut chan<- iutil.Command
	inbox      chan *iutil.Event

	instances    map[string]*iutil.Event
	lastDeployed map[string][]byte
	chain        iutil.ChainState
}

var _ iutil.Plugin = (*Plugin)(nil)

// New constructs a new Caddy plugin.
func New(logger *slog.Logger, cfg Config) *Plugin {
	inboxSize := cfg.InboxSize
	if inboxSize <= 0 {
		inboxSize = defaultInboxSize
	}

	return &Plugin{
		logger:       logger,
		cfg:          cfg,
		inbox:        make(chan *iutil.Event, inboxSize),
		instances:    make(map[string]*iutil.Event),
		lastDeployed: make(map[string][]byte),
	}
}

// Name identifies this plugin in the ievent chain.
func (p *Plugin) Name() string {
	return name
}

// Wants declares which instance lifecycle actions this plugin monitors.
func (p *Plugin) Wants() []iutil.Want {
	enrichment := iutil.EnrichedInstance | iutil.EnrichedInstanceWithInterfaces

	actions := []string{
		"instance-created",
		"instance-started",
		"instance-stopped",
		"instance-deleted",
		"instance-updated",
		"instance-renamed",
	}

	wants := make([]iutil.Want, 0, len(actions)+1)
	for _, a := range actions {
		wants = append(wants, iutil.Want{
			Action:   a,
			Enrich:   enrichment,
			Debounce: true,
		})
	}

	wants = append(wants, iutil.Want{
		Action:   iutil.ActionSweepEnd,
		Debounce: false,
	})

	return wants
}

// Setup initializes the plugin with chain resources and the Incus connection.
func (p *Plugin) Setup(args iutil.SetupArgs) error {
	p.next = args.Next
	p.commandIn = args.CommandIn
	p.commandOut = args.CommandOut
	p.conn = args.Conn

	return nil
}

// Handle forwards the event along the chain and enqueues it for background deployment.
func (p *Plugin) Handle(ev *iutil.Event) {
	if ev.Err() != nil {
		p.next(ev)

		return
	}

	select {
	case p.inbox <- ev:
	default:
		p.logger.Warn("Caddy plugin inbox full; dropping event", "action", ev.Action(), "name", ev.Name())
	}

	p.next(ev)
}

// Run processes queued events and reconciles Caddyfiles until the context is canceled.
func (p *Plugin) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case cmd, ok := <-p.commandIn:
			if !ok {
				return nil
			}

			if cmd.Action == iutil.CommandDrain {
				p.drainInbox(ctx)
				p.commandOut <- cmd

				return nil
			}

		case ev, ok := <-p.inbox:
			if !ok {
				return nil
			}

			p.processEvent(ctx, ev)
		}
	}
}

func (p *Plugin) processEvent(ctx context.Context, ev *iutil.Event) {
	if ev.Action() == iutil.ActionDisconnected {
		p.chain = iutil.ChainCold

		return
	}

	p.chain = ev.ChainState()

	if ev.Action() == iutil.ActionSweepEnd {
		p.chain = iutil.ChainWarm
		p.reconcile(ctx)

		return
	}

	if ev.OldName() != "" {
		delete(p.instances, ev.ProjectName()+"/"+ev.OldName())
	}

	key := ev.ProjectName() + "/" + ev.Name()
	had := p.instances[key] != nil
	inst := ev.Instance()
	relevant := inst != nil && inst.Running() && p.hasTargetLabels(inst)

	if !relevant {
		delete(p.instances, key)
		if !had {
			return
		}
	} else {
		p.instances[key] = ev
	}

	if p.chain != iutil.ChainWarm {
		return
	}

	p.reconcile(ctx)
}

func (p *Plugin) hasTargetLabels(inst *iutil.Instance) bool {
	if inst == nil {
		return false
	}

	for _, target := range p.cfg.Targets {
		prefix := "user.label." + target.Label + "."
		for k := range inst.Config() {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
	}

	for _, target := range p.cfg.OSTargets {
		prefix := "user.label." + target.Label + "."
		for k := range inst.Config() {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
	}

	return false
}

func (p *Plugin) drainInbox(ctx context.Context) {
	for {
		select {
		case ev := <-p.inbox:
			p.processEvent(ctx, ev)
		default:
			return
		}
	}
}

func (p *Plugin) reconcile(ctx context.Context) {
	instances := make([]*iutil.Event, 0, len(p.instances))
	for _, ev := range p.instances {
		instances = append(instances, ev)
	}

	for _, target := range p.cfg.Targets {
		vhosts := extractVhosts(target.Label, instances)

		globalTmpl := p.cfg.GlobalTemplates[target.Label]
		content, err := render(vhosts, p.cfg.TemplatesDir, globalTmpl)
		if err != nil {
			p.logger.Error("rendering Caddyfile", "label", target.Label, "err", err)

			continue
		}

		sum := sha256.Sum256(content)
		targetKey := target.Project + "/" + target.Instance

		last := p.lastDeployed[targetKey]
		if bytes.Equal(last, sum[:]) {
			continue
		}

		if p.conn == nil {
			continue
		}

		err = deploy(ctx, p.logger, p.conn, target, p.cfg.CaddyfilePath, content)
		if err != nil {
			p.logger.Error("deploying Caddyfile", "target", targetKey, "label", target.Label, "err", err)

			continue
		}

		p.lastDeployed[targetKey] = sum[:]
	}

	for _, target := range p.cfg.OSTargets {
		vhosts := extractVhosts(target.Label, instances)

		globalTmpl := p.cfg.GlobalTemplates[target.Label]
		content, err := render(vhosts, p.cfg.TemplatesDir, globalTmpl)
		if err != nil {
			p.logger.Error("rendering Caddyfile for OS target", "label", target.Label, "path", target.Path, "err", err)

			continue
		}

		sum := sha256.Sum256(content)
		targetKey := "os:" + target.Label + ":" + target.Path

		last := p.lastDeployed[targetKey]
		if bytes.Equal(last, sum[:]) {
			continue
		}

		err = deployOS(ctx, p.logger, target, content)
		if err != nil {
			p.logger.Error("deploying Caddyfile to OS path", "label", target.Label, "path", target.Path, "err", err)

			continue
		}

		p.lastDeployed[targetKey] = sum[:]
	}
}
