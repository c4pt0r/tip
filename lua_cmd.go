package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type LuaRunFileCmd struct{}

func (cmd LuaRunFileCmd) Name() string {
	return ".lua-eval-file"
}

func (cmd LuaRunFileCmd) Description() string {
	return "Execute a Lua file"
}

func (cmd LuaRunFileCmd) Usage() string {
	return ".lua-eval-file <filename> <arg> <arg> <arg>..."
}

func (cmd LuaRunFileCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
	filename := args[0]
	scriptContent, err := FetchLuaScriptContent(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read Lua script: %v\n", err)
		os.Exit(1)
	}
	return ExecuteLuaScript(string(scriptContent), args[1:], os.Stdout)
}

type LuaCmd struct{}

func (cmd LuaCmd) Name() string {
	return ".lua-eval"
}

func (cmd LuaCmd) Description() string {
	return "Execute a Lua script"
}

func (cmd LuaCmd) Usage() string {
	return ".lua-eval \"<script>\""
}

// ParseLuaScriptAndArgs parses the raw input to extract the Lua script and arguments
func (cmd LuaCmd) parseLuaScriptAndArgs(rawInput string) (string, []string, error) {
	// Find the script part (everything between the first pair of quotes)
	re := regexp.MustCompile(`\.lua-eval\s+"((?:[^"\\]|\\.)*)"`)
	matches := re.FindStringSubmatch(rawInput)
	if len(matches) < 2 {
		return "", nil, fmt.Errorf("invalid script format: script must be enclosed in quotes")
	}

	// Get the script content
	script := matches[1]
	script = strings.Replace(script, `\"`, `"`, -1)

	// Get the position after the script
	scriptEndPos := strings.Index(rawInput, matches[0]) + len(matches[0])
	argsPart := strings.TrimSpace(rawInput[scriptEndPos:])

	// Parse arguments properly handling quotes
	var parsedArgs []string
	var currentArg strings.Builder
	var inQuotes bool
	var escapeNext bool

	for i := 0; i < len(argsPart); i++ {
		char := argsPart[i]

		if escapeNext {
			currentArg.WriteByte(char)
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' {
			if inQuotes {
				inQuotes = false
			} else {
				inQuotes = true
			}
			continue
		}

		if char == ' ' && !inQuotes {
			if currentArg.Len() > 0 {
				parsedArgs = append(parsedArgs, currentArg.String())
				currentArg.Reset()
			}
			continue
		}

		currentArg.WriteByte(char)
	}

	if currentArg.Len() > 0 {
		parsedArgs = append(parsedArgs, currentArg.String())
	}

	return script, parsedArgs, nil
}

func (cmd LuaCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: .lua-eval \"<script>\" <args> <args> <args> ...")
	}

	// Parse the script and arguments
	script, parsedArgs, err := cmd.parseLuaScriptAndArgs(rawInput)
	if err != nil {
		return err
	}

	// Execute the Lua script
	return ExecuteLuaScript(script, parsedArgs, resultWriter)
}
