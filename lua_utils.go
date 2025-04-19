package main

import (
	"fmt"
	"io"
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

	resultWriter.Write([]byte("Lua script executed successfully\n"))
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
