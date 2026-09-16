package main

import (
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

type TargetFlag = cli.FlagBase[[]caddy.Target, cli.NoConfig, targetValue]

type targetValue struct {
	destination *[]caddy.Target
	hasBeenSet  bool
}

func (t targetValue) Create(val []caddy.Target, p *[]caddy.Target, _ cli.NoConfig) cli.Value {
	*p = val

	return &targetValue{
		destination: p,
	}
}

func (t targetValue) ToString(val []caddy.Target) string {
	strs := make([]string, 0, len(val))
	for _, target := range val {
		strs = append(strs, target.String())
	}

	return strings.Join(strs, ", ")
}

func (t *targetValue) Set(val string) error {
	targets, err := caddy.ParseTargets(val)
	if err != nil {
		return err
	}

	if !t.hasBeenSet {
		*t.destination = []caddy.Target{}
		t.hasBeenSet = true
	}

	*t.destination = append(*t.destination, targets...)

	return nil
}

func (t *targetValue) Get() any {
	if t.destination == nil {
		return nil
	}

	return *t.destination
}

func (t *targetValue) String() string {
	if t.destination == nil {
		return ""
	}

	return t.ToString(*t.destination)
}
