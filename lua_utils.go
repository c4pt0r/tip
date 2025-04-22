package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Global Lua state
var (
	globalLuaState *lua.LState
	luaStateMutex  sync.Mutex
)

// InitializeLuaState initializes the global Lua state if it hasn't been initialized yet
func InitializeLuaState() {
	luaStateMutex.Lock()
	defer luaStateMutex.Unlock()

	if globalLuaState == nil {
		globalLuaState = lua.NewState()
		registerSQLFunctions(globalLuaState)
		registerHTTPFunctions(globalLuaState)
	}
}

// CloseLuaState closes the global Lua state
func CloseLuaState() {
	luaStateMutex.Lock()
	defer luaStateMutex.Unlock()

	if globalLuaState != nil {
		globalLuaState.Close()
		globalLuaState = nil
	}
}

// GetLuaState returns the global Lua state
func GetLuaState() *lua.LState {
	luaStateMutex.Lock()
	defer luaStateMutex.Unlock()

	if globalLuaState == nil {
		InitializeLuaState()
	}

	return globalLuaState
}

// FetchLuaScriptContent reads Lua script content from either a file or URL
func FetchLuaScriptContent(source string) ([]byte, error) {
	// Check if the source is a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch URL: %w", err)
		}
		defer resp.Body.Close()
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		return content, nil
	}

	// If not a URL, treat as a local file
	content, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read Lua script file: %w", err)
	}
	return content, nil
}

// ExecuteLuaScript executes a Lua script with the given arguments
func ExecuteLuaScript(script string, args []string, resultWriter io.Writer) error {
	L := GetLuaState()

	// Create arg table for Lua script
	argTable := L.NewTable()
	for i, arg := range args {
		argTable.RawSetInt(i+1, lua.LString(arg))
	}
	L.SetGlobal("args", argTable)

	// Execute the Lua script
	if err := L.DoString(script); err != nil {
		return fmt.Errorf("lua execution error: %v", err)
	}

	// Get the return value from the Lua script
	returnValue := L.Get(-1)

	// Print the return value
	log.Println("Lua script executed successfully, return value:", returnValue)
	return nil
}

// registerSQLFunctions registers SQL functions in the Lua state
func registerSQLFunctions(L *lua.LState) {
	sqlTable := L.NewTable()

	// Register query function
	sqlTable.RawSetString("query", L.NewFunction(func(L *lua.LState) int {
		query := L.ToString(1)
		result := L.NewTable()
		result.RawSetString("ok", lua.LBool(true))
		result.RawSetString("error", lua.LString(""))

		conn := GetDB()
		if conn == nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString("database connection is not available, please connect first using .connect command"))
			L.Push(result)
			return 1
		}

		rows, err := conn.Query(query)
		if err != nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString(err.Error()))
			L.Push(result)
			return 1
		}
		defer rows.Close()

		// Get column types
		columns, err := rows.Columns()
		if err != nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString(err.Error()))
			L.Push(result)
			return 1
		}

		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		// Create result table
		resultTable := L.NewTable()

		// Add header row
		headerRow := L.NewTable()
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
				L.Push(result)
				return 1
			}

			// Create row table
			rowTable := L.NewTable()
			for i, v := range values {
				var luaValue lua.LValue
				switch val := v.(type) {
				case []byte:
					luaValue = lua.LString(string(val))
				case nil:
					// nil is not a valid Lua value, so we use LNil
					luaValue = lua.LNil
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

		L.Push(result)
		return 1
	}))

	// Register execute function
	sqlTable.RawSetString("execute", L.NewFunction(func(L *lua.LState) int {
		query := L.ToString(1)
		result := L.NewTable()
		result.RawSetString("ok", lua.LBool(true))
		result.RawSetString("error", lua.LString(""))

		conn := GetDB()
		if conn == nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString("database connection is not available, please connect first using .connect command"))
			L.Push(result)
			return 1
		}

		res, err := conn.Exec(query)
		if err != nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString(err.Error()))
			L.Push(result)
			return 1
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString(err.Error()))
			L.Push(result)
			return 1
		}

		lastInsertId, err := res.LastInsertId()
		if err != nil {
			result.RawSetString("ok", lua.LBool(false))
			result.RawSetString("error", lua.LString(err.Error()))
			L.Push(result)
			return 1
		}

		// Set the data in the result object
		result.RawSetString("rows_affected", lua.LNumber(rowsAffected))
		result.RawSetString("last_insert_id", lua.LNumber(lastInsertId))

		L.Push(result)
		return 1
	}))

	L.SetGlobal("sql", sqlTable)
}

// registerHTTPFunctions registers HTTP functions in the Lua state
func registerHTTPFunctions(L *lua.LState) {
	httpTable := L.NewTable()

	// Register do function
	httpTable.RawSetString("fetch", L.NewFunction(func(L *lua.LState) int {
		// Get parameters
		method := L.ToString(1)
		url := L.ToString(2)
		headers := L.ToTable(3)
		body := L.ToString(4)
		callback := L.ToFunction(5)

		// Create HTTP client
		client := &http.Client{}

		// Create request
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Add headers if provided
		if headers != nil {
			headers.ForEach(func(k, v lua.LValue) {
				req.Header.Add(k.String(), v.String())
			})
		}

		// If callback is provided, do async request
		if callback != nil {
			// Create new goroutine for async execution
			go func() {
				// Execute request
				resp, err := client.Do(req)
				if err != nil {
					// Schedule callback execution in main thread
					L.Push(callback)
					L.Push(lua.LBool(false))
					L.Push(lua.LString(err.Error()))
					L.CallByParam(lua.P{
						Fn:      callback,
						NRet:    0,
						Protect: true,
					}, lua.LBool(false), lua.LString(err.Error()))
					return
				}
				defer resp.Body.Close()

				// Read response body
				respBody, err := io.ReadAll(resp.Body)
				if err != nil {
					L.CallByParam(lua.P{
						Fn:      callback,
						NRet:    0,
						Protect: true,
					}, lua.LBool(false), lua.LString(err.Error()))
					return
				}

				// Create response table
				responseTable := L.NewTable()
				responseTable.RawSetString("status_code", lua.LNumber(resp.StatusCode))
				responseTable.RawSetString("body", lua.LString(string(respBody)))

				// Create headers table
				headersTable := L.NewTable()
				for k, v := range resp.Header {
					if len(v) > 0 {
						headersTable.RawSetString(k, lua.LString(v[0]))
					}
				}
				responseTable.RawSetString("headers", headersTable)

				// Schedule callback execution in main thread
				L.CallByParam(lua.P{
					Fn:      callback,
					NRet:    0,
					Protect: true,
				}, lua.LBool(true), responseTable)
			}()

			return 0
		}

		// Synchronous execution (no callback)
		resp, err := client.Do(req)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Create response table
		responseTable := L.NewTable()
		responseTable.RawSetString("status_code", lua.LNumber(resp.StatusCode))
		responseTable.RawSetString("body", lua.LString(string(respBody)))

		// Create headers table
		headersTable := L.NewTable()
		for k, v := range resp.Header {
			if len(v) > 0 {
				headersTable.RawSetString(k, lua.LString(v[0]))
			}
		}
		responseTable.RawSetString("headers", headersTable)

		L.Push(lua.LBool(true))
		L.Push(responseTable)
		return 2
	}))

	L.SetGlobal("http", httpTable)
}
