package main

import (
	"database/sql"
	"io"
	"strings"
)

type SystemCmd interface {
	Handle(db *sql.DB, args []string, resultWriter io.Writer) error
	Name() string
}

var (
	RegisteredSystemCmds = []SystemCmd{
		HelpCmd{},
		VerCmd{},
	}
)

func SystemCmdNames() []string {
	names := make([]string, len(RegisteredSystemCmds))
	for i, cmd := range RegisteredSystemCmds {
		names[i] = cmd.Name()
	}
	return names
}

type HelpCmd struct{}

func (cmd HelpCmd) Name() string {
	return ".help"
}

func (cmd HelpCmd) Handle(db *sql.DB, args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("Help for tip:\n"))
	for _, cmd := range RegisteredSystemCmds {
		resultWriter.Write([]byte(cmd.Name() + "\n"))
	}
	return nil
}

type VerCmd struct{}

func (cmd VerCmd) Name() string {
	return ".ver"
}

func (cmd VerCmd) Handle(db *sql.DB, args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("tip version: " + Version + "\n"))
	return nil
}

func handleCmd(db *sql.DB, line string, resultWriter io.Writer) error {
	line = strings.TrimSpace(line)
	cmdName := strings.Split(line, " ")[0]
	params := strings.Split(line, " ")[1:]
	for _, cmd := range RegisteredSystemCmds {
		if cmd.Name() == cmdName {
			return cmd.Handle(db, params, resultWriter)
		}
	}
	return nil
}
