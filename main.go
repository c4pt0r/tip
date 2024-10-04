package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/olekukonko/tablewriter"
	"github.com/pelletier/go-toml"
	"github.com/peterh/liner"
	"golang.org/x/term"
)

type OutputFormat int

const (
	Plain OutputFormat = iota
	JSON
	Table
)

func (f OutputFormat) String() string {
	return [...]string{"plain", "json", "table"}[f]
}

func parseOutputFormat(format string) OutputFormat {
	switch format {
	case "json":
		return JSON
	case "table":
		return Table
	default:
		return Plain
	}
}

// Load configuration from a file
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

// Load configuration from environment variables or .env file
func loadConfigFromEnv() (string, string, string, string, string, error) {
	err := godotenv.Load(".env") // Optionally load .env file
	if err != nil {
		log.Println("No .env file found")
	}
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

// RowResult represents a single row of query results
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

func isTerminal() bool {
	fd := int(os.Stdin.Fd())
	return term.IsTerminal(fd)
}

func executeSQL(db *sql.DB, query string) ([]RowResult, bool, int64, error) {
	var output []RowResult
	var hasRows bool
	var affectedRows int64

	ok, err := isQuery(query)
	if err != nil {
		return nil, false, 0, fmt.Errorf("failed to parse SQL: %w", err)
	}

	if ok {
		rows, err := db.Query(query)
		if err != nil {
			return nil, false, 0, fmt.Errorf("failed to execute SQL: %w", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return nil, false, 0, fmt.Errorf("failed to get column info: %w", err)
		}

		results := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))
		for i := range results {
			pointers[i] = &results[i]
		}

		for rows.Next() {
			hasRows = true
			if err := rows.Scan(pointers...); err != nil {
				return nil, false, 0, fmt.Errorf("failed to read data: %w", err)
			}
			rowData := RowResult{
				colNames:  cols,
				colValues: make([]interface{}, len(cols)),
			}
			for i := range cols {
				rowData.colValues[i] = results[i]
			}
			output = append(output, rowData)
		}
	} else {
		result, err := db.Exec(query)
		if err != nil {
			return nil, false, 0, fmt.Errorf("failed to execute SQL: %w", err)
		}
		affectedRows, err = result.RowsAffected()
		if err != nil {
			return nil, false, 0, fmt.Errorf("failed to get affected rows: %w", err)
		}
	}

	return output, hasRows, affectedRows, nil
}

func repl(db *sql.DB, outputFormat OutputFormat) {
	line := liner.NewLiner()
	defer func() {
		line.Close()
		// show cursor
		fmt.Print("\033[?25h")
	}()

	historyFile := filepath.Join(os.Getenv("HOME"), ".tip/history")
	if f, err := os.Open(historyFile); err == nil {
		line.ReadHistory(f)
		f.Close()
	}

	var queryBuilder string

	for {
		var prompt string
		if isTerminal() {
			var curDB string
			db.QueryRow("SELECT DATABASE()").Scan(&curDB)
			if curDB == "" {
				curDB = "(none)"
			}
			if queryBuilder == "" {
				prompt = fmt.Sprintf("%s> ", curDB)
			} else {
				prompt = fmt.Sprintf("%s>> ", curDB)
			}
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			break
		}

		trimmedInput := strings.TrimSpace(input)
		queryBuilder += input + " "

		// Check if the trimmed input ends with a semicolon
		if len(trimmedInput) > 0 && trimmedInput[len(trimmedInput)-1] == ';' {
			startTime := time.Now() // Start timing the query execution

			output, hasRows, affectedRows, err := executeSQL(db, queryBuilder)
			if err != nil {
				log.Println(err)
				queryBuilder = "" // Reset the query builder
				continue
			}
			execTime := time.Since(startTime)

			printResults(output, outputFormat, hasRows, execTime, affectedRows)
			line.AppendHistory(queryBuilder) // Append to history after successful execution
			queryBuilder = ""                // Reset the query builder after execution
		}
	}

	if f, err := os.Create(historyFile); err != nil {
		log.Printf("Error writing history file: %v", err)
	} else {
		line.WriteHistory(f)
		f.Close()
	}
}

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

func printResults(output []RowResult, outputFormat OutputFormat, hasRows bool, execTime time.Duration, affectedRows int64) {
	if outputFormat == JSON {
		jsonOutput, err := json.Marshal(output)
		if err != nil {
			log.Printf("Failed to marshal JSON: %v", err)
			return
		}
		fmt.Println(string(jsonOutput))
	} else if outputFormat == Plain {
		for _, row := range output {
			for i, col := range row.colNames {
				val := row.colValues[i]
				fmt.Printf("%s: %s ", col, formatValue(val))
			}
			fmt.Println()
		}
	} else if outputFormat == Table {
		if len(output) == 0 {
			return
		}
		cols := output[0].colNames
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader(cols)
		for _, row := range output {
			rowData := make([]string, len(cols))
			for i := range cols {
				val := row.colValues[i]
				rowData[i] = formatValue(val)
			}
			table.Append(rowData)
		}
		table.Render()
	} else {
		log.Fatal("Invalid output format: " + outputFormat.String())
	}

	if !hasRows && outputFormat != JSON {
		fmt.Println("OK")
	}

	if showExecDetails {
		fmt.Fprintf(os.Stderr, "-----\n")
		fmt.Fprintf(os.Stderr, "Execution time: %s\n", execTime)
		if hasRows {
			fmt.Fprintf(os.Stderr, "Rows in result: %d\n", len(output))
		}
		if affectedRows > 0 {
			fmt.Fprintf(os.Stderr, "Affected rows: %d\n", affectedRows)
		}
	}
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

var (
	Version         = "dev"
	showExecDetails = false
)

func main() {
	// Command-line flags
	host := flag.String("h", "127.0.0.1", "TiDB Serverless hostname")
	port := flag.String("p", "4000", "TiDB port")
	user := flag.String("u", "root", "TiDB username")
	pass := flag.String("P", "", "TiDB password")
	dbName := flag.String("d", "test", "TiDB database")
	configFile := flag.String("c", getDefaultConfigFilePath(), "Path to configuration file")
	outputFormat := flag.String("o", "table", "Output format: plain, table(default) or json")
	execSQL := flag.String("e", "", "Execute SQL statement and exit")
	version := flag.Bool("v", false, "Display version information")
	verbose := flag.Bool("vv", false, "Display execution details")
	flag.Parse()

	showExecDetails = *verbose

	// Load config from file if provided
	if *configFile != "" {
		config, err := loadConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to read config file: %v", err)
		}
		*host = config["host"]
		*port = config["port"]
		*user = config["user"]
		*pass = config["password"]
		*dbName = config["database"]
	} else {
		// Otherwise, try loading from environment variables
		envHost, envPort, envUser, envPass, defaultDatabase, err := loadConfigFromEnv()
		if err == nil {
			if envHost != "" {
				*host = envHost
			}
			if envPort != "" {
				*port = envPort
			}
			if envUser != "" {
				*user = envUser
			}
			if envPass != "" {
				*pass = envPass
			}
			if defaultDatabase != "" {
				*dbName = defaultDatabase
			}
		}
	}

	mysql.RegisterTLSConfig("tidb", &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: *host,
	})

	// Database connection string
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&tls=tidb", *user, *pass, *host, *port, *dbName)

	// Connect to TiDB
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to TiDB: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(100)

	// Check if -e flag is provided
	if *execSQL != "" {
		startTime := time.Now() // Start timing the query execution

		output, hasRows, affectedRows, err := executeSQL(db, *execSQL)
		if err != nil {
			log.Fatalf("Failed to execute SQL: %v", err)
		}

		execTime := time.Since(startTime)
		printResults(output, parseOutputFormat(*outputFormat), hasRows, execTime, affectedRows)

		return
	}
	if *version {
		if info, ok := debug.ReadBuildInfo(); ok && Version == "dev" {
			Version = info.Main.Version
		}
		fmt.Printf("tip version: %s\n", Version)
		os.Exit(0)
	}
	// Execute queries in REPL mode
	repl(db, parseOutputFormat(*outputFormat))
}
