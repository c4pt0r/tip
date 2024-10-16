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
		OutputFormatCmd{}, // Add the new command to the list
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

type OutputFormatCmd struct{}

func (cmd OutputFormatCmd) Name() string {
	return ".output_format"
}

func (cmd OutputFormatCmd) Handle(dbPtr **sql.DB, args []string, resultWriter io.Writer) error {
	if len(args) == 0 {
		// If no arguments, print the current output format and available options
		current := *globalOutputFormat
		options := []string{"json", "table", "plain", "csv"}
		formattedOptions := make([]string, len(options))

		for i, opt := range options {
			if opt == current.String() {
				formattedOptions[i] = "[" + opt + "]"
			} else {
				formattedOptions[i] = opt
			}
		}

		resultWriter.Write([]byte(fmt.Sprintf("Current output format: %s\n", current)))
		resultWriter.Write([]byte(fmt.Sprintf("Available formats: %s\n", strings.Join(formattedOptions, " "))))
		return nil
	}

	if len(args) != 1 {
		return fmt.Errorf("usage: .output_format <format>")
	}

	format := parseOutputFormat(args[0])
	if format == Plain && args[0] != "plain" {
		return fmt.Errorf("invalid format: %s", args[0])
	}

	// Update the global outputFormat variable
	*globalOutputFormat = format

	resultWriter.Write([]byte(fmt.Sprintf("Output format set to: %s\n", format)))
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
