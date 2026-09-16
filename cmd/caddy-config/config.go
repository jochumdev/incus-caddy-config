package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

const (
	defaultHTTPAddr      = ":9153"
	defaultDataDir       = "/var/lib/caddy-config"
	defaultSecretsDir    = "/run/secrets"
	defaultCaddyfilePath = "/config/Caddyfile"

	defaultDebounceWindow = 250 * time.Millisecond
	defaultWorkers        = 16
	defaultReadTimeout    = 10 * time.Second
	defaultProjectDelay   = 30 * time.Second
	defaultReadDelay      = 5 * time.Second
)

type config struct {
	IncusURL   string
	Token      string
	DataDir    string
	SecretsDir string
	ClientCert string
	ClientKey  string
	Restricted bool
	Remote     string
	UseRemote  bool

	Projects  []string
	Targets   []caddy.Target
	OSTargets []caddy.Target

	CaddyfilePath string
	TemplatesDir  string

	DebounceWindow time.Duration
	HTTPAddr       string
	Exclude        []string
	Log            string
	Pprof          bool

	Workers      int
	ReadTimeout  time.Duration
	ProjectDelay time.Duration
	ReadDelay    time.Duration
}

type mainActionArgs struct {
	IncusURL   string
	Token      string
	DataDir    string
	SecretsDir string
	ClientCert string
	ClientKey  string
	Restricted bool
	Remote     string
	UseRemote  bool

	Projects  []string
	Targets   []caddy.Target
	OSTargets []caddy.Target

	CaddyfilePath string
	TemplatesDir  string

	DebounceWindow time.Duration
	HTTPAddr       string
	Exclude        []string
	Log            string
	Pprof          bool

	Workers      int
	ReadTimeout  time.Duration
	ProjectDelay time.Duration
	ReadDelay    time.Duration
}

func newConfig() *config {
	return &config{
		DataDir:        defaultDataDir,
		SecretsDir:     defaultSecretsDir,
		CaddyfilePath:  defaultCaddyfilePath,
		HTTPAddr:       defaultHTTPAddr,
		DebounceWindow: defaultDebounceWindow,
		Workers:        defaultWorkers,
		ReadTimeout:    defaultReadTimeout,
		ProjectDelay:   defaultProjectDelay,
		ReadDelay:      defaultReadDelay,
	}
}

func (c *config) validate() (*mainActionArgs, error) {
	if len(c.Targets) == 0 && len(c.OSTargets) == 0 {
		return nil, errors.New("at least one --caddy-instance or --os-path must be specified")
	}

	for _, t := range c.Targets {
		instance, ok := t.Flag("instance")
		if !ok || instance == "" {
			return nil, fmt.Errorf("invalid --caddy-instance %q: missing required flag \"instance\"", t.String())
		}

		project, ok := t.Flag("project")
		if !ok || project == "" {
			return nil, fmt.Errorf("invalid --caddy-instance %q: missing required flag \"project\"", t.String())
		}
	}

	for _, t := range c.OSTargets {
		path, ok := t.Flag("path")
		if !ok || path == "" {
			return nil, fmt.Errorf("invalid --os-path %q: missing required flag \"path\"", t.String())
		}
	}

	return &mainActionArgs{
		IncusURL:       c.IncusURL,
		Token:          c.Token,
		DataDir:        c.DataDir,
		SecretsDir:     c.SecretsDir,
		ClientCert:     c.ClientCert,
		ClientKey:      c.ClientKey,
		Restricted:     c.Restricted,
		Remote:         c.Remote,
		UseRemote:      c.UseRemote,
		Projects:       c.Projects,
		Targets:        c.Targets,
		OSTargets:      c.OSTargets,
		CaddyfilePath:  c.CaddyfilePath,
		TemplatesDir:   c.TemplatesDir,
		DebounceWindow: c.DebounceWindow,
		HTTPAddr:       c.HTTPAddr,
		Exclude:        c.Exclude,
		Log:            c.Log,
		Pprof:          c.Pprof,
		Workers:        c.Workers,
		ReadTimeout:    c.ReadTimeout,
		ProjectDelay:   c.ProjectDelay,
		ReadDelay:      c.ReadDelay,
	}, nil
}

func (a *mainActionArgs) endpoint() string {
	if a.IncusURL != "" {
		return a.IncusURL
	}

	if a.UseRemote {
		if a.Remote != "" {
			return "remote:" + a.Remote
		}

		return "remote:default"
	}

	return ""
}

func (a *mainActionArgs) redactedToken() string {
	if a.Token != "" {
		return fmt.Sprintf("<redacted-(%d)>", len(a.Token))
	}

	return ""
}
