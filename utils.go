package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/pelletier/go-toml"
	"golang.org/x/term"
)

// Common variables
var (
	cachedDBNames     []string
	cachedTableNames  = make(map[string][]string)
	cachedColumnNames = make(map[string][]string)
)

var KEYWORDS = []string{
	"USE", "SELECT", "FROM", "WHERE", "JOIN", "ON", "GROUP BY", "ORDER BY",
	"LIMIT", "OFFSET", "AS", "IS", "NULL", "NOT", "IN", "BETWEEN", "LIKE",
	"SHOW", "DATABASES", "TABLES", "COLUMNS", "INDEXES", "STATISTICS",
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "GRANT", "REVOKE",
	"UPDATE", "SET", "WHERE", "ON", "AND", "OR", "XOR", "NOT", "EXISTS",
}

// Output format types
type OutputFormat int

const (
	Plain OutputFormat = iota
	JSON
	Table
	CSV
)

func (f OutputFormat) String() string {
	return [...]string{"plain", "json", "table", "csv"}[f]
}

func parseOutputFormat(format string) OutputFormat {
	switch format {
	case "json":
		return JSON
	case "table":
		return Table
	case "csv":
		return CSV
	default:
		return Plain
	}
}

// Terminal utilities
func isTerminal() bool {
	fd := int(os.Stdin.Fd())
	return term.IsTerminal(fd)
}

// Value formatting utilities
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case nil:
		return "NULL"
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	case string:
		return v
	case []byte:
		return string(v)
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatCSVValue(val interface{}) string {
	switch v := val.(type) {
	case nil:
		return ""
	case bool:
		return fmt.Sprintf("%t", v)
	case int, int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	case string:
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(v, "\"", "\"\""))
	case []byte:
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(string(v), "\"", "\"\""))
	case time.Time:
		return fmt.Sprintf("\"%s\"", v.Format("2006-01-02 15:04:05"))
	default:
		return fmt.Sprintf("\"%v\"", v)
	}
}

// Config utilities
func loadConfigFromFile(configPath string) (map[string]string, error) {
	config := make(map[string]string)
	file, err := os.ReadFile(configPath)
	if err != nil {
		return config, err
	}

	err = toml.Unmarshal(file, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func loadConfigFromEnv() (string, string, string, string, string, error) {
	godotenv.Load(".env") // Optionally load .env file
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USERNAME")
	password := os.Getenv("DB_PASSWORD")

	defaultDatabase := os.Getenv("DB_DATABASE")
	if defaultDatabase == "" {
		defaultDatabase = "test"
	}
	return host, port, user, password, defaultDatabase, nil
}

func getDefaultConfigFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	configFile := filepath.Join(homeDir, ".tip/config.toml")
	if _, err := os.Stat(configFile); err != nil {
		return ""
	}
	return configFile
}

// Result row structure
type RowResult struct {
	colNames  []string
	colValues []interface{}
}

// MarshalJSON customizes the JSON serialization of RowResult
func (r RowResult) MarshalJSON() ([]byte, error) {
	converted := make(map[string]interface{})
	for i, col := range r.colNames {
		val := r.colValues[i]
		if byteVal, ok := val.([]byte); ok {
			converted[col] = string(byteVal)
		} else {
			converted[col] = val
		}
	}
	return json.Marshal(converted)
}

// Get databases and tables
func getDatabases(db *sql.DB) ([]string, error) {
	if len(cachedDBNames) > 0 {
		return cachedDBNames, nil
	}
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err == nil {
			databases = append(databases, dbName)
		}
	}
	cachedDBNames = databases
	return databases, nil
}

func getTableNames(db *sql.DB, dbName string) ([]string, error) {
	if cachedTableNames[dbName] != nil {
		return cachedTableNames[dbName], nil
	}
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err == nil {
			tables = append(tables, tableName)
		}
	}
	cachedTableNames[dbName] = tables
	return tables, nil
}

func getAllColumnNames(db *sql.DB, dbName string) ([]string, error) {
	if cachedColumnNames[dbName] != nil {
		return cachedColumnNames[dbName], nil
	}
	rows, err := db.Query("SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ?", dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columnNames []string
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err == nil {
			columnNames = append(columnNames, columnName)
		}
	}
	cachedColumnNames[dbName] = columnNames
	return columnNames, nil
}

// Connection info and handling
type ConnInfo struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

var (
	globalDB     *sql.DB
	globalDBLock sync.RWMutex
)

// GetDB returns the current global database connection
func GetDB() *sql.DB {
	globalDBLock.RLock()
	defer globalDBLock.RUnlock()
	return globalDB
}

// SetDB sets the global database connection
func SetDB(db *sql.DB) {
	globalDBLock.Lock()
	defer globalDBLock.Unlock()
	globalDB = db
}