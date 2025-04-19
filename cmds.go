package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type SystemCmd interface {
	Handle(args []string, rawInput string, resultWriter io.Writer) error
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
		LuaCmd{},
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
			return cmd.Handle(params, line, resultWriter)
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

func (cmd HelpCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
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

func (cmd VerCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
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

func (cmd RefreshCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
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

func (cmd ConnectCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: .connect <host> <port> <user> <password> [database]")
	}

	host := args[0]
	port := args[1]
	user := args[2]
	pass := args[3]
	dbName := ""
	if len(args) > 4 {
		dbName = args[4]
	} else if GetLastUsedDB() != "" {
		// If no database is specified and we have a last used database, use it
		dbName = GetLastUsedDB()
	} else {
		dbName = "test" // Default database if no last used database
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

func (cmd OutputFormatCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
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

type LuaCmd struct {
	state *lua.LState
}

func (cmd LuaCmd) Name() string {
	return ".lua-eval"
}

func (cmd LuaCmd) Description() string {
	return "Execute a Lua script"
}

func (cmd LuaCmd) Usage() string {
	return ".lua-eval \"<script>\""
}

func (cmd LuaCmd) Handle(args []string, rawInput string, resultWriter io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: .lua-eval \"<script>\" <args> <args> <args> ...")
	}

	// Find the script part (everything between the first pair of quotes)
	re := regexp.MustCompile(`\.lua-eval\s+"((?:[^"\\]|\\.)*)"`)
	matches := re.FindStringSubmatch(rawInput)
	if len(matches) < 2 {
		return fmt.Errorf("invalid script format: script must be enclosed in quotes")
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

	// Initialize Lua state if not already done
	if cmd.state == nil {
		cmd.state = lua.NewState()
		defer cmd.state.Close()
	}

	// Create arg table for Lua script
	argTable := cmd.state.NewTable()
	for i, arg := range parsedArgs {
		argTable.RawSetInt(i+1, lua.LString(arg))
	}
	cmd.state.SetGlobal("args", argTable)

	funcMap := map[string]interface{}{
		"query": func(query string) (*lua.LTable, error) {
			// Create a Result object
			result := cmd.state.NewTable()
			result.RawSetString("ok", lua.LBool(true))
			result.RawSetString("error", lua.LString(""))

			conn := GetDB()
			if conn == nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString("database connection is not available, please connect first using .connect command"))
				return result, nil
			}

			rows, err := conn.Query(query)
			if err != nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString(err.Error()))
				return result, nil
			}
			defer rows.Close()

			// Get column types
			columns, err := rows.Columns()
			if err != nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString(err.Error()))
				return result, nil
			}

			// Create a slice of interface{} to hold the values
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range columns {
				valuePtrs[i] = &values[i]
			}

			// Create result table
			resultTable := cmd.state.NewTable()

			// Add header row
			headerRow := cmd.state.NewTable()
			for i, col := range columns {
				headerRow.RawSetInt(i+1, lua.LString(col))
			}
			// Add data rows
			rowIndex := 1
			for rows.Next() {
				err := rows.Scan(valuePtrs...)
				if err != nil {
					result.RawSetString("ok", lua.LBool(false))
					result.RawSetString("error", lua.LString(err.Error()))
					return result, nil
				}

				// Create row table
				rowTable := cmd.state.NewTable()
				for i, v := range values {
					var luaValue lua.LValue
					switch val := v.(type) {
					case []byte:
						luaValue = lua.LString(string(val))
					case nil:
						luaValue = lua.LString("NULL")
					case int64:
						luaValue = lua.LNumber(val)
					case float64:
						luaValue = lua.LNumber(val)
					case bool:
						luaValue = lua.LBool(val)
					case time.Time:
						luaValue = lua.LString(val.Format("2006-01-02 15:04:05"))
					default:
						luaValue = lua.LString(fmt.Sprintf("%v", val))
					}
					rowTable.RawSetInt(i+1, luaValue)
				}
				resultTable.RawSetInt(rowIndex, rowTable)
				rowIndex++
			}

			// Set the data in the result object
			result.RawSetString("data", resultTable)
			result.RawSetString("columns", headerRow)
			result.RawSetString("row_count", lua.LNumber(rowIndex-1))

			return result, nil
		},
		"execute": func(query string) (*lua.LTable, error) {
			// Create a Result object
			result := cmd.state.NewTable()
			result.RawSetString("ok", lua.LBool(true))
			result.RawSetString("error", lua.LString(""))

			conn := GetDB()
			if conn == nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString("database connection is not available, please connect first using .connect command"))
				return result, nil
			}

			res, err := conn.Exec(query)
			if err != nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString(err.Error()))
				return result, nil
			}

			rowsAffected, err := res.RowsAffected()
			if err != nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString(err.Error()))
				return result, nil
			}

			lastInsertId, err := res.LastInsertId()
			if err != nil {
				result.RawSetString("ok", lua.LBool(false))
				result.RawSetString("error", lua.LString(err.Error()))
				return result, nil
			}

			// Set the data in the result object
			result.RawSetString("rows_affected", lua.LNumber(rowsAffected))
			result.RawSetString("last_insert_id", lua.LNumber(lastInsertId))

			return result, nil
		},
	}

	sqlTable := cmd.state.NewTable()
	for name, fn := range funcMap {
		fnCopy := fn
		sqlTable.RawSetString(name, cmd.state.NewFunction(func(L *lua.LState) int {
			query := L.ToString(1)
			switch f := fnCopy.(type) {
			case func(string) (*lua.LTable, error):
				result, err := f(query)
				if err != nil {
					// Create a Result object with error
					errorResult := L.NewTable()
					errorResult.RawSetString("ok", lua.LBool(false))
					errorResult.RawSetString("error", lua.LString(err.Error()))
					L.Push(errorResult)
					return 1
				}
				L.Push(result)
			case func(string) (string, error):
				result, err := f(query)
				if err != nil {
					// Create a Result object with error
					errorResult := L.NewTable()
					errorResult.RawSetString("ok", lua.LBool(false))
					errorResult.RawSetString("error", lua.LString(err.Error()))
					L.Push(errorResult)
					return 1
				}
				// Create a Result object with success
				successResult := L.NewTable()
				successResult.RawSetString("ok", lua.LBool(true))
				successResult.RawSetString("data", lua.LString(result))
				L.Push(successResult)
			default:
				// Create a Result object with error
				errorResult := L.NewTable()
				errorResult.RawSetString("ok", lua.LBool(false))
				errorResult.RawSetString("error", lua.LString("internal error: unsupported function type"))
				L.Push(errorResult)
			}
			return 1
		}))
	}
	cmd.state.SetGlobal("sql", sqlTable)

	// Execute the Lua script
	if err := cmd.state.DoString(script); err != nil {
		return fmt.Errorf("lua execution error: %v", err)
	}

	resultWriter.Write([]byte("Lua script executed successfully\n"))
	return nil
}
