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

func executeSQL(db *sql.DB, query string) (bool, []RowResult, bool, int64, error) {
	var output []RowResult
	var hasRows bool
	var affectedRows int64

	isQ, err := isQuery(query)
	if err != nil {
		return false, nil, false, 0, fmt.Errorf("failed to parse SQL: %w", err)
	}

	if isQ {
		rows, err := db.Query(query)
		if err != nil {
			return false, nil, false, 0, fmt.Errorf("failed to execute SQL: %w", err)
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return false, nil, false, 0, fmt.Errorf("failed to get column info: %w", err)
		}

		results := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))
		for i := range results {
			pointers[i] = &results[i]
		}

		for rows.Next() {
			hasRows = true
			if err := rows.Scan(pointers...); err != nil {
				return false, nil, false, 0, fmt.Errorf("failed to read data: %w", err)
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
			return false, nil, false, 0, fmt.Errorf("failed to execute SQL: %w", err)
		}
		affectedRows, err = result.RowsAffected()
		if err != nil {
			return false, nil, false, 0, fmt.Errorf("failed to get affected rows: %w", err)
		}
	}

	return isQ, output, hasRows, affectedRows, nil
}

func repl(db *sql.DB, outputFormat OutputFormat) {
	if isTerminal() {
		showExecDetails = true
	}
	line := liner.NewLiner()
	defer func() {
		line.Close()
		// show cursor
		fmt.Print("\033[?25h")
	}()
	historyFile := filepath.Join(os.Getenv("HOME"), ".tip/history")
	// ensure directory exists
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(historyFile), 0o755)
	}
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
				prompt = fmt.Sprintf("%s>>> ", curDB)
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
			line.AppendHistory(queryBuilder)
			isQ, output, hasRows, affectedRows, err := executeSQL(db, queryBuilder)
			if err != nil {
				log.Println(err)
				queryBuilder = "" // Reset the query builder
				continue
			}
			execTime := time.Since(startTime)
			printResults(isQ, output, outputFormat, hasRows, execTime, affectedRows)
			queryBuilder = "" // Reset the query builder after execution
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

func printResults(isQ bool, output []RowResult, outputFormat OutputFormat, hasRows bool, execTime time.Duration, affectedRows int64) {
	if outputFormat == JSON {
		if len(output) == 0 {
			if !isQ {
				fmt.Println("{\"status\": \"OK\", \"affected_rows\": " + fmt.Sprintf("%d", affectedRows) + "}")
			} else {
				fmt.Println("[]")
			}
			goto I
		}
		jsonOutput, err := json.Marshal(output)
		if err != nil {
			log.Printf("Failed to marshal JSON: %v", err)
			return
		}
		fmt.Println(string(jsonOutput))
	} else if outputFormat == Plain {
		if len(output) == 0 {
			if !isQ {
				fmt.Println("OK, affected_rows:", affectedRows)
			} else {
				fmt.Println("(empty result)")
			}
			goto I
		}
		for _, row := range output {
			for i, col := range row.colNames {
				val := row.colValues[i]
				fmt.Printf("%s: %s ", col, formatValue(val))
			}
			fmt.Println()
		}
	} else if outputFormat == Table {
		if len(output) == 0 {
			if !isQ {
				fmt.Println("OK, affected_rows:", affectedRows)
			} else {
				fmt.Println("(empty result)")
			}
			goto I
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
I:
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
	host := flag.String("host", "", "TiDB Serverless hostname")
	port := flag.String("port", "", "TiDB port")
	user := flag.String("u", "", "TiDB username")
	pass := flag.String("p", "", "TiDB password")
	dbName := flag.String("d", "", "TiDB database")
	configFile := flag.String("c", getDefaultConfigFilePath(), "Path to configuration file")
	outputFormat := flag.String("o", "table", "Output format: plain, table(default) or json")
	execSQL := flag.String("e", "", "Execute SQL statement and exit")
	version := flag.Bool("version", false, "Display version information")
	verbose := flag.Bool("v", false, "Display execution details")
	flag.Parse()

	showExecDetails = *verbose

	// Load config from environment variables
	envHost, envPort, envUser, envPass, defaultDatabase, _ := loadConfigFromEnv()

	// Load config from file if provided
	if *configFile != "" {
		config, err := loadConfigFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to read config file: %v", err)
		}
		if *host == "" && config["host"] != "" {
			*host = config["host"]
		}
		if *port == "" && config["port"] != "" {
			*port = config["port"]
		}
		if *user == "" && config["user"] != "" {
			*user = config["user"]
		}
		if *pass == "" && config["password"] != "" {
			*pass = config["password"]
		}
		if *dbName == "" && config["database"] != "" {
			*dbName = config["database"]
		}
	}

	// Use environment variables if command line and config file are not set
	if *host == "" {
		*host = envHost
	}
	if *port == "" {
		*port = envPort
	}
	if *user == "" {
		*user = envUser
	}
	if *pass == "" {
		*pass = envPass
	}
	if *dbName == "" {
		*dbName = defaultDatabase
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

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping TiDB: %v", err)
	}

	// Check if -e flag is provided
	if *execSQL != "" {
		startTime := time.Now() // Start timing the query execution

		isQ, output, hasRows, affectedRows, err := executeSQL(db, *execSQL)
		if err != nil {
			log.Fatalf("Failed to execute SQL: %v", err)
		}

		execTime := time.Since(startTime)
		printResults(isQ, output, parseOutputFormat(*outputFormat), hasRows, execTime, affectedRows)

		return
	}
	if *version {
		if info, ok := debug.ReadBuildInfo(); ok && Version == "dev" {
			Version = info.Main.Version
		}
		fmt.Printf("tip version: %s\n", Version)
		os.Exit(0)
	}
	repl(db, parseOutputFormat(*outputFormat))
}
