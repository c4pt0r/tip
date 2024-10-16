package main

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
)

type SystemCmd interface {
	Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error
	Name() string
}

var (
	RegisteredSystemCmds = []SystemCmd{
		HelpCmd{},
		VerCmd{},
		RefreshCmd{},
		ConnectCmd{},
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

func (cmd HelpCmd) Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error {
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

func (cmd VerCmd) Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("tip version: " + Version + "\n"))
	return nil
}

type RefreshCmd struct{}

func (cmd RefreshCmd) Name() string {
	return ".refresh_completion"
}

func (cmd RefreshCmd) Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("not impl yet\n"))
	return nil
}

type ConnectCmd struct{}

func (cmd ConnectCmd) Name() string {
	return ".connect"
}

func (cmd ConnectCmd) Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: .connect <host> <port> <user> <password> [database]")
	}

	host := args[0]
	port := args[1]
	user := args[2]
	pass := args[3]
	dbName := "test"
	if len(args) > 4 {
		dbName = args[4]
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4", user, pass, host, port, dbName)

	// Try connecting with TLS
	db, err := connectWithRetry(dsn, host, true)
	if err != nil {
		fmt.Println("Attempting connection without TLS...")
		// Try connecting without TLS
		db, err = connectWithRetry(dsn, host, false)
		if err != nil {
			return fmt.Errorf("failed to connect to TiDB: %v", err)
		}
	}

	*dbPtr = db
	resultWriter.Write([]byte("Connected successfully.\n"))
	return nil
}

func handleCmd(dbPtr **sql.DB, line string, resultWriter io.Writer) error {
	line = strings.TrimSpace(line)
	cmdName := strings.Split(line, " ")[0]
	params := strings.Split(line, " ")[1:]
	for _, cmd := range RegisteredSystemCmds {
		if cmd.Name() == cmdName {
			return cmd.Handle(dbPtr, params, resultWriter)
		}
	}
	return nil
}
