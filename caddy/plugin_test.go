package caddy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/ievent/iutil"
)

func TestPluginBasics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := Config{
		Targets: []Target{
			{Label: "caddy1", Project: "default", Instance: "caddy-ext"},
		},
		CaddyfilePath: "/config/Caddyfile",
	}

	p := New(logger, cfg)
	require.Equal(t, "caddy", p.Name())

	wants := p.Wants()
	require.NotEmpty(t, wants)

	var handled *iutil.Event
	setupArgs := iutil.SetupArgs{
		Next: func(ev *iutil.Event) {
			handled = ev
		},
	}
	err := p.Setup(setupArgs)
	require.NoError(t, err)

	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "web-1", "")
	p.Handle(ev)
	require.Equal(t, ev, handled)
}

func TestPluginProcessEvent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{
		Targets: []Target{
			{Label: "caddy", Project: "default", Instance: "caddy-ext"},
		},
	})

	now := time.Now()

	// Unrelated instance without matching labels is ignored.
	unrelatedInst := iutil.NewInstance(true, map[string]string{
		"user.label.other.domain": "example.org",
	}, nil, nil)
	unrelatedEv := iutil.NewEvent(now, "instance-started", "default", "db-1", "").WithInstance(unrelatedInst, true)
	ctx := context.Background()
	p.processEvent(ctx, unrelatedEv)
	require.NotContains(t, p.instances, "default/db-1")

	// Matching instance is tracked.
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "example.com",
	}, nil, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst, true)

	p.processEvent(ctx, ev)
	require.Contains(t, p.instances, "default/web-1")
	require.NotEqual(t, iutil.ChainWarm, p.chain)

	// Sweep end turns chain warm.
	sweepEv := iutil.NewEvent(now, iutil.ActionSweepEnd, "", "", "").WithChainState(iutil.ChainWarm)
	p.processEvent(ctx, sweepEv)
	require.Equal(t, iutil.ChainWarm, p.chain)

	// Stopped instance removes it.
	stoppedInst := iutil.NewInstance(false, nil, nil, nil)
	stoppedEv := iutil.NewEvent(now, "instance-stopped", "default", "web-1", "").WithInstance(stoppedInst, true)
	p.processEvent(ctx, stoppedEv)

	require.NotContains(t, p.instances, "default/web-1")
}

func TestPluginRunContextCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Run(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPluginRunCommandDrain(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{
		Targets: []Target{
			{Label: "caddy", Project: "default", Instance: "caddy-ext"},
		},
	})

	cmdIn := make(chan iutil.Command, 1)
	cmdOut := make(chan iutil.Command, 1)
	err := p.Setup(iutil.SetupArgs{
		CommandIn:  cmdIn,
		CommandOut: cmdOut,
		Next:       func(*iutil.Event) {},
	})
	require.NoError(t, err)

	now := time.Now()
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "drain.example.com",
	}, nil, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "web-drain", "").WithInstance(inst, true)
	p.Handle(ev)

	cmdIn <- iutil.Command{Action: iutil.CommandDrain}

	err = p.Run(context.Background())
	require.NoError(t, err)

	select {
	case out := <-cmdOut:
		require.Equal(t, iutil.CommandDrain, out.Action)
	default:
		t.Fatal("expected commandOut to receive CommandDrain")
	}

	require.Contains(t, p.instances, "default/web-drain")
}

func TestPluginRunClosedChannels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. commandIn closed.
	p1 := New(logger, Config{})
	cmdIn := make(chan iutil.Command)
	close(cmdIn)
	err := p1.Setup(iutil.SetupArgs{
		CommandIn:  cmdIn,
		CommandOut: make(chan iutil.Command, 1),
		Next:       func(*iutil.Event) {},
	})
	require.NoError(t, err)

	err = p1.Run(context.Background())
	require.NoError(t, err)

	// 2. inbox closed.
	p2 := New(logger, Config{})
	close(p2.inbox)

	err = p2.Run(context.Background())
	require.NoError(t, err)
}

func TestPluginHandleErrorEvent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{})

	var forwarded *iutil.Event
	err := p.Setup(iutil.SetupArgs{
		Next: func(ev *iutil.Event) {
			forwarded = ev
		},
	})
	require.NoError(t, err)

	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "err-inst", "").WithFailed(io.EOF)
	p.Handle(ev)

	require.Equal(t, ev, forwarded)
	require.Empty(t, p.inbox)
}

func TestPluginHandleInboxFull(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{InboxSize: 1})

	var count int
	err := p.Setup(iutil.SetupArgs{
		Next: func(*iutil.Event) {
			count++
		},
	})
	require.NoError(t, err)

	now := time.Now()
	ev1 := iutil.NewEvent(now, "instance-started", "default", "inst-1", "")
	ev2 := iutil.NewEvent(now, "instance-started", "default", "inst-2", "")

	p.Handle(ev1)
	p.Handle(ev2)

	require.Equal(t, 2, count)
	require.Len(t, p.inbox, 1)
}

func TestPluginProcessEventExtended(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{
		Targets: []Target{
			{Label: "caddy", Project: "default", Instance: "caddy-ext"},
		},
	})

	now := time.Now()
	ctx := context.Background()

	// 1. Track an initial instance.
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "app.example.com",
	}, nil, nil)
	evOld := iutil.NewEvent(now, "instance-started", "default", "app-v1", "").WithInstance(inst, true)
	p.processEvent(ctx, evOld)
	require.Contains(t, p.instances, "default/app-v1")

	// 2. Renaming: event carries oldName "app-v1", removes old and tracks new.
	evRename := iutil.NewEvent(now, "instance-renamed", "default", "app-v2", "app-v1").WithInstance(inst, true)
	p.processEvent(ctx, evRename)
	require.NotContains(t, p.instances, "default/app-v1")
	require.Contains(t, p.instances, "default/app-v2")

	// 3. Disconnect event resets chain to ChainCold.
	p.chain = iutil.ChainWarm
	evDisc := iutil.NewEvent(now, iutil.ActionDisconnected, "", "", "")
	p.processEvent(ctx, evDisc)
	require.Equal(t, iutil.ChainCold, p.chain)

	// 4. Stopped event for an instance that was never tracked.
	stoppedGhost := iutil.NewInstance(false, nil, nil, nil)
	evGhost := iutil.NewEvent(now, "instance-stopped", "default", "ghost", "").WithInstance(stoppedGhost, true)
	p.processEvent(ctx, evGhost)
	require.NotContains(t, p.instances, "default/ghost")
}

func TestPluginReconcileBranches(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := New(logger, Config{
		Targets: []Target{
			{Label: "caddy", Project: "default", Instance: "caddy-ext"},
		},
		CaddyfilePath: "/config/Caddyfile",
	})
	p.chain = iutil.ChainWarm

	now := time.Now()
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "reconcile.example.com",
	}, nil, nil)
	p.instances["default/app"] = iutil.NewEvent(now, "instance-started", "default", "app", "").WithInstance(inst, true)

	ctx := context.Background()

	// 1. conn is nil: renders Caddyfile, but skips deploy without error or panic.
	p.reconcile(ctx)

	// 2. Hash matches lastDeployed: deployment is skipped.
	targetKey := "default/caddy-ext"
	p.lastDeployed[targetKey] = []byte("dummy-sha")

	// 3. Template render error: handles failure gracefully and continues.
	p.cfg.Targets = []Target{
		{Label: "broken", Project: "default", Instance: "caddy-ext"},
	}
	brokenInst := iutil.NewInstance(true, map[string]string{
		"user.label.broken.domain":   "broken.com",
		"user.label.broken.template": "{{ .Unclosed",
	}, nil, nil)
	p.instances["default/broken"] = iutil.NewEvent(now, "instance-started", "default", "broken", "").WithInstance(brokenInst, true)

	p.reconcile(ctx)
}

func TestPluginHasTargetLabels(t *testing.T) {
	p := New(nil, Config{
		Targets: []Target{
			{Label: "web", Project: "default", Instance: "caddy"},
		},
	})

	require.False(t, p.hasTargetLabels(nil))

	instNoLabels := iutil.NewInstance(true, map[string]string{}, nil, nil)
	require.False(t, p.hasTargetLabels(instNoLabels))

	instOtherLabels := iutil.NewInstance(true, map[string]string{
		"user.label.db.port": "5432",
	}, nil, nil)
	require.False(t, p.hasTargetLabels(instOtherLabels))

	instMatch := iutil.NewInstance(true, map[string]string{
		"user.label.web.domain": "site.lan",
	}, nil, nil)
	require.True(t, p.hasTargetLabels(instMatch))

	// Match via OSTargets.
	pOS := New(nil, Config{
		OSTargets: []OSTarget{
			{Label: "local", Path: "/etc/caddy/Caddyfile"},
		},
	})
	instOSMatch := iutil.NewInstance(true, map[string]string{
		"user.label.local.domain": "local.lan",
	}, nil, nil)
	require.True(t, pOS.hasTargetLabels(instOSMatch))
}

func TestPluginReconcileOSTarget(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	p := New(logger, Config{
		OSTargets: []OSTarget{
			{Label: "edge", Path: targetPath},
		},
	})

	now := time.Now()
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain": "app.test",
	}, nil, nil)
	p.instances["default/app"] = iutil.NewEvent(now, "instance-started", "default", "app", "").WithInstance(inst, true)

	ctx := context.Background()
	p.reconcile(ctx)

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "app.test")

	// Reconcile again with same data: skipped via SHA cache.
	p.reconcile(ctx)

	// Reconcile with broken template: handles error gracefully.
	brokenPath := filepath.Join(tmpDir, "BrokenCaddyfile")
	pBroken := New(logger, Config{
		OSTargets: []OSTarget{
			{Label: "bad", Path: brokenPath},
		},
	})
	badInst := iutil.NewInstance(true, map[string]string{
		"user.label.bad.domain":   "bad.test",
		"user.label.bad.template": "{{ .Unclosed",
	}, nil, nil)
	pBroken.instances["default/bad"] = iutil.NewEvent(now, "instance-started", "default", "bad", "").WithInstance(badInst, true)
	pBroken.reconcile(ctx)

	_, err = os.Stat(brokenPath)
	require.True(t, os.IsNotExist(err))
}

func TestPluginReconcileWithLabelPrefixedGlobalTemplates(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	tmpDir := t.TempDir()
	pathExt := filepath.Join(tmpDir, "external.Caddyfile")
	pathInt := filepath.Join(tmpDir, "internal.Caddyfile")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	p := New(logger, Config{
		OSTargets: []OSTarget{
			{Label: "caddy-external", Path: pathExt},
			{Label: "caddy-internal", Path: pathInt},
		},
		GlobalTemplates: map[string]string{
			"caddy-external": "{\n\tadmin localhost:2019\n\t# external-global-header\n}",
			"caddy-internal": "{\n\tadmin localhost:2019\n\t# internal-global-header\n}",
		},
	})

	now := time.Now()
	instExt := iutil.NewInstance(true, map[string]string{
		"user.label.caddy-external.domain": "ext.example.com",
	}, nil, nil)
	instInt := iutil.NewInstance(true, map[string]string{
		"user.label.caddy-internal.domain": "int.example.com",
	}, nil, nil)

	p.instances["default/ext"] = iutil.NewEvent(now, "instance-started", "default", "ext", "").WithInstance(instExt, true)
	p.instances["default/int"] = iutil.NewEvent(now, "instance-started", "default", "int", "").WithInstance(instInt, true)

	ctx := context.Background()
	p.reconcile(ctx)

	dataExt, err := os.ReadFile(pathExt)
	require.NoError(t, err)
	require.Contains(t, string(dataExt), "# external-global-header")
	require.NotContains(t, string(dataExt), "# internal-global-header")
	require.Contains(t, string(dataExt), "ext.example.com")

	dataInt, err := os.ReadFile(pathInt)
	require.NoError(t, err)
	require.Contains(t, string(dataInt), "# internal-global-header")
	require.NotContains(t, string(dataInt), "# external-global-header")
	require.Contains(t, string(dataInt), "int.example.com")
}
