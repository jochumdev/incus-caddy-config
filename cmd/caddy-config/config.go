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

	Projects       []string
	CaddyInstances []string

	CaddyfilePath      string
	CustomTemplatesDir string

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

	Projects []string
	Targets  []caddy.Target

	CaddyfilePath      string
	CustomTemplatesDir string

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
	if len(c.CaddyInstances) == 0 {
		return nil, errors.New("at least one --caddy-instance must be specified (format: 'label:project:instance')")
	}

	targets := make([]caddy.Target, 0, len(c.CaddyInstances))
	for _, entry := range c.CaddyInstances {
		target, err := caddy.ParseTarget(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid --caddy-instance %q: %w", entry, err)
		}

		targets = append(targets, target)
	}

	return &mainActionArgs{
		IncusURL:           c.IncusURL,
		Token:              c.Token,
		DataDir:            c.DataDir,
		SecretsDir:         c.SecretsDir,
		ClientCert:         c.ClientCert,
		ClientKey:          c.ClientKey,
		Restricted:         c.Restricted,
		Remote:             c.Remote,
		UseRemote:          c.UseRemote,
		Projects:           c.Projects,
		Targets:            targets,
		CaddyfilePath:      c.CaddyfilePath,
		CustomTemplatesDir: c.CustomTemplatesDir,
		DebounceWindow:     c.DebounceWindow,
		HTTPAddr:           c.HTTPAddr,
		Exclude:            c.Exclude,
		Log:                c.Log,
		Pprof:              c.Pprof,
		Workers:            c.Workers,
		ReadTimeout:        c.ReadTimeout,
		ProjectDelay:       c.ProjectDelay,
		ReadDelay:          c.ReadDelay,
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
