package main

import (
	"fmt"
	"io"
	"strings"
)

type SystemCmd interface {
	Handle(args []string, resultWriter io.Writer) error
	Name() string
	Description() string
	Usage() string
}

var (
	RegisteredSystemCmds = []SystemCmd{
		HelpCmd{},
		VerCmd{},
		RefreshCmd{},
		ConnectCmd{},
		OutputFormatCmd{},
		AskCmd{},
	}
)

func SystemCmdNames() []string {
	names := make([]string, len(RegisteredSystemCmds))
	for i, cmd := range RegisteredSystemCmds {
		names[i] = cmd.Name()
	}
	return names
}

func handleCmd(line string, resultWriter io.Writer) error {
	line = strings.TrimSpace(line)
	cmdName := strings.Split(line, " ")[0]
	params := strings.Split(line, " ")[1:]
	for _, cmd := range RegisteredSystemCmds {
		if cmd.Name() == cmdName {
			return cmd.Handle(params, resultWriter)
		}
	}
	resultWriter.Write([]byte("Unknown command: " + cmdName + ", use .help for help\n"))
	return nil
}

type HelpCmd struct{}

func (cmd HelpCmd) Name() string {
	return ".help"
}

func (cmd HelpCmd) Description() string {
	return "Display help information for all available commands"
}

func (cmd HelpCmd) Usage() string {
	return ".help"
}

func (cmd HelpCmd) Handle(args []string, resultWriter io.Writer) error {
	for _, cmd := range RegisteredSystemCmds {
		resultWriter.Write([]byte(cmd.Name() + " - " + cmd.Description() + "- Usage: " + cmd.Usage() + "\n"))
	}
	return nil
}

type VerCmd struct{}

func (cmd VerCmd) Name() string {
	return ".ver"
}

func (cmd VerCmd) Description() string {
	return "Display the current version of tip"
}

func (cmd VerCmd) Usage() string {
	return ".ver"
}

func (cmd VerCmd) Handle(args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("tip version: " + Version + "\n"))
	return nil
}

type RefreshCmd struct{}

func (cmd RefreshCmd) Name() string {
	return ".refresh_completion"
}

func (cmd RefreshCmd) Description() string {
	return "Refresh completion (not implemented yet)"
}

func (cmd RefreshCmd) Usage() string {
	return ".refresh_completion"
}

func (cmd RefreshCmd) Handle(args []string, resultWriter io.Writer) error {
	resultWriter.Write([]byte("not impl yet\n"))
	return nil
}

type ConnectCmd struct{}

func (cmd ConnectCmd) Name() string {
	return ".connect"
}

func (cmd ConnectCmd) Description() string {
	return "Connect to a TiDB database"
}

func (cmd ConnectCmd) Usage() string {
	return ".connect <host> <port> <user> <password> [database]"
}

func (cmd ConnectCmd) Handle(args []string, resultWriter io.Writer) error {
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

	connInfo := ConnInfo{
		Host:     host,
		Port:     port,
		User:     user,
		Password: pass,
		Database: dbName,
	}

	err := connectToDatabase(connInfo)
	if err != nil {
		return fmt.Errorf("failed to connect to TiDB: %v", err)
	}

	resultWriter.Write([]byte("Connected successfully.\n"))
	return nil
}

type OutputFormatCmd struct{}

func (cmd OutputFormatCmd) Name() string {
	return ".output_format"
}

func (cmd OutputFormatCmd) Description() string {
	return "Set or display the current output format"
}

func (cmd OutputFormatCmd) Usage() string {
	return ".output_format [format]"
}

func (cmd OutputFormatCmd) Handle(args []string, resultWriter io.Writer) error {
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

		resultWriter.Write([]byte(fmt.Sprintf("%s\n", strings.Join(formattedOptions, " "))))
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
