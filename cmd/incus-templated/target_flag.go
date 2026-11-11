package main

import (
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jochumdev/incus-templated/templated"
)

type TargetFlag = cli.FlagBase[[]templated.Target, cli.NoConfig, targetValue]

type targetValue struct {
	destination *[]templated.Target
	hasBeenSet  bool
}

func (t targetValue) Create(val []templated.Target, p *[]templated.Target, _ cli.NoConfig) cli.Value {
	*p = val

	return &targetValue{
		destination: p,
	}
}

func (t targetValue) ToString(val []templated.Target) string {
	strs := make([]string, 0, len(val))
	for _, target := range val {
		strs = append(strs, target.String())
	}

	return strings.Join(strs, ", ")
}

func (t *targetValue) Set(val string) error {
	targets, err := templated.ParseTargets(val)
	if err != nil {
		return err
	}

	if !t.hasBeenSet {
		*t.destination = []templated.Target{}
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
