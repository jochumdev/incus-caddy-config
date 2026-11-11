package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/jochumdev/incus-templated/caddy"
	"github.com/jochumdev/incus-templated/templated"
)

const (
	defaultHTTPAddr   = ":9153"
	defaultDataDir    = "/var/lib/incus-templated"
	defaultSecretsDir = "/run/secrets"

	defaultDebounceWindow = 250 * time.Millisecond
	defaultWorkers        = 16
	defaultReadTimeout    = 10 * time.Second
	defaultProjectDelay   = 30 * time.Second
	defaultReadDelay      = 5 * time.Second
)

type config struct {
	IncusURL          string
	ServerFingerprint string
	Token             string
	DataDir           string
	SecretsDir        string
	ClientCert        string
	ClientKey         string
	Restricted        bool
	Remote            string
	UseRemote         bool

	Projects []string
	Targets  []templated.Target

	ConfigFile   string
	TemplatesDir string

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
	IncusURL          string
	ServerFingerprint string
	Token             string
	DataDir           string
	SecretsDir        string
	ClientCert        string
	ClientKey         string
	Restricted        bool
	Remote            string
	UseRemote         bool

	Projects []string
	Targets  []templated.Target

	TemplatesDir string

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
		HTTPAddr:       defaultHTTPAddr,
		DebounceWindow: defaultDebounceWindow,
		Workers:        defaultWorkers,
		ReadTimeout:    defaultReadTimeout,
		ProjectDelay:   defaultProjectDelay,
		ReadDelay:      defaultReadDelay,
	}
}

// presets fill in what a target leaves empty, keyed by its preset= name.
var presets = map[string]templated.Target{
	"caddy": caddy.Preset(),
}

// configFile is the --config file: targets too long for a flag, and where their templates live.
type configFile struct {
	TemplatesDir string             `yaml:"templates_dir"`
	Targets      []templated.Target `yaml:"targets"`
}

func (c *config) validate() (*mainActionArgs, error) {
	templatesDir := c.TemplatesDir
	targets := []templated.Target{}

	if c.ConfigFile != "" {
		b, err := os.ReadFile(c.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("reading --config: %w", err)
		}

		var file configFile

		err = yaml.Unmarshal(b, &file)
		if err != nil {
			return nil, fmt.Errorf("parsing --config %s: %w", c.ConfigFile, err)
		}

		if templatesDir == "" {
			templatesDir = file.TemplatesDir
		}

		targets = append(targets, file.Targets...)
	}

	targets = append(targets, c.Targets...)
	if len(targets) == 0 {
		return nil, errors.New("at least one --target must be specified, by flag or in --config")
	}

	written := map[string]string{}

	for i := range targets {
		t := &targets[i]

		if t.Preset != "" {
			p, ok := presets[t.Preset]
			if !ok {
				return nil, fmt.Errorf("invalid target %q: unknown preset %q", t.String(), t.Preset)
			}

			if t.Template == "" {
				t.Template, t.Text = p.Template, p.Text
			}

			// The preset path is where the file sits inside its instance, never a host path.
			if t.Path == "" && t.Instance != "" {
				t.Path = p.Path
			}

			// nil is unset; an empty list is the target turning the step off.
			if t.Format == nil {
				t.Format = p.Format
			}

			if t.Validate == nil {
				t.Validate = p.Validate
			}

			if t.Reload == nil {
				t.Reload = p.Reload
			}
		}

		if t.Label == "" {
			return nil, fmt.Errorf("invalid target %q: missing label", t.String())
		}

		if t.Instance != "" && t.Project == "" {
			return nil, fmt.Errorf("invalid target %q: instance needs a project", t.String())
		}

		if t.Path == "" {
			return nil, fmt.Errorf("invalid target %q: missing path", t.String())
		}

		other, ok := written[t.Key()]
		if ok {
			return nil, fmt.Errorf("invalid target %q: writes the same file as %q", t.String(), other)
		}

		written[t.Key()] = t.String()

		if t.Text != "" {
			continue
		}

		if t.Template == "" {
			return nil, fmt.Errorf("invalid target %q: missing template or preset", t.String())
		}

		path := t.Template
		if !filepath.IsAbs(path) && templatesDir != "" {
			path = filepath.Join(templatesDir, path)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: reading template: %w", t.String(), err)
		}

		t.Text = string(b)
	}

	return &mainActionArgs{
		IncusURL:          c.IncusURL,
		ServerFingerprint: c.ServerFingerprint,
		Token:             c.Token,
		DataDir:           c.DataDir,
		SecretsDir:        c.SecretsDir,
		ClientCert:        c.ClientCert,
		ClientKey:         c.ClientKey,
		Restricted:        c.Restricted,
		Remote:            c.Remote,
		UseRemote:         c.UseRemote,
		Projects:          c.Projects,
		Targets:           targets,
		TemplatesDir:      templatesDir,
		DebounceWindow:    c.DebounceWindow,
		HTTPAddr:          c.HTTPAddr,
		Exclude:           c.Exclude,
		Log:               c.Log,
		Pprof:             c.Pprof,
		Workers:           c.Workers,
		ReadTimeout:       c.ReadTimeout,
		ProjectDelay:      c.ProjectDelay,
		ReadDelay:         c.ReadDelay,
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
