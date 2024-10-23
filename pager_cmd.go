package main

import "io"

type PagerCmd struct{}

func (cmd PagerCmd) Name() string {
	return ".pager"
}

func (cmd PagerCmd) Description() string {
	return "Toggle pager mode"
}

func (cmd PagerCmd) Usage() string {
	return ".pager [vim|less|...|off]"
}

func (cmd PagerCmd) Handle(args []string, resultWriter io.Writer) error {
	return nil
}
