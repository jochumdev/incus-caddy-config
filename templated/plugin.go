// Package templated renders Go templates from Incus instance labels and deploys the result.
package templated

import (
	"context"
	"log/slog"
	"maps"
	"text/template"

	"github.com/lxc/incus-compose/iclient"
	"github.com/lxc/incus-compose/ievent/iutil"
)

const name = "templated"

const defaultInboxSize = 256

// Config configures the plugin.
type Config struct {
	Targets []Target

	// Funcs are added to every target's template, over the generic ones.
	Funcs     template.FuncMap
	InboxSize int
}

// Plugin consumes enriched instance events and deploys each target's rendered template.
type Plugin struct {
	logger *slog.Logger
	cfg    Config
	conn   *iclient.Connection

	next       iutil.Next
	commandIn  <-chan iutil.Command
	commandOut chan<- iutil.Command
	inbox      chan *iutil.Event

	instances map[string]*iutil.Event
	chain     iutil.ChainState

	deploy *deployState
}

var _ iutil.Plugin = (*Plugin)(nil)

// New constructs a new plugin.
func New(logger *slog.Logger, cfg Config) *Plugin {
	inboxSize := cfg.InboxSize
	if inboxSize <= 0 {
		inboxSize = defaultInboxSize
	}

	return &Plugin{
		logger:    logger,
		cfg:       cfg,
		inbox:     make(chan *iutil.Event, inboxSize),
		instances: make(map[string]*iutil.Event),
		deploy:    newDeployState(),
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
		"instance-shutdown",
		"instance-restarted",
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
		p.logger.Warn("inbox full; dropping event", "action", ev.Action(), "name", ev.Name())
	}

	p.next(ev)
}

// Wait blocks until all in-flight deployments have completed.
func (p *Plugin) Wait() {
	p.deploy.wg.Wait()
}

// Run processes queued events and reconciles targets until the context is canceled.
func (p *Plugin) Run(ctx context.Context) error {
	defer cancelAll(p.deploy.cancels)

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

		case res := <-p.deploy.done:
			p.deploy.lastDeployed[res.targetKey] = res.hash
			delete(p.deploy.cancels, res.targetKey)
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
	config := maps.Collect(inst.Config())
	for _, target := range p.cfg.Targets {
		if HasLabel(config, target.Label) {
			return true
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

	reconcileDeployments(ctx, p.deploy, p.logger, p.conn, p.cfg.Targets, p.cfg.Funcs, instances)
}
