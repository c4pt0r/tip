package main

import (
	"bufio"
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

func executeSQL(db *sql.DB, query string, resultIOWriter ResultIOWriter) (bool, []RowResult, bool, int64, error) {
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
			if resultIOWriter != nil {
				if err := resultIOWriter.Write([]RowResult{rowData}); err != nil {
					return false, nil, false, 0, fmt.Errorf("failed to write data: %w", err)
				}
			} else {
				output = append(output, rowData)
			}

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

var globalOutputFormat *OutputFormat

func repl(db *sql.DB, outputFormat *OutputFormat) {
	if isTerminal() {
		showExecDetails = true
	}
	line := liner.NewLiner()
	defer func() {
		line.Close()
		// show cursor
		fmt.Print("\033[?25h")
	}()

	var curDB string
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
	completer := func(line string, pos int) (head string, completions []string, tail string) {
		databases, err := getDatabases(db)
		if err != nil {
			log.Println(err)
			return
		}
		tables, err := getTableNames(db, curDB)
		if err != nil {
			log.Println(err)
			return
		}
		cols, err := getAllColumnNames(db, curDB)
		if err != nil {
			log.Println(err)
			return
		}
		words := strings.Fields(line[:pos])
		lastWord := ""
		if len(words) > 0 {
			lastWord = strings.ToLower(words[len(words)-1])
		}
		keywords := append(KEYWORDS, append(databases, append(tables, cols...)...)...)
		keywords = append(keywords, SystemCmdNames()...)

		for _, item := range keywords {
			if strings.HasPrefix(strings.ToLower(item), lastWord) {
				completions = append(completions, item)
			}
		}
		if len(completions) == 0 {
			return
		}
		if pos > 0 && line[pos-1] == ' ' {
			completions = []string{}
			head = line[:pos]
			tail = line[pos:]
		} else {
			head = line[:pos-len(lastWord)]
			tail = line[pos:]
		}
		return
	}
	line.SetWordCompleter(completer)
	line.SetTabCompletionStyle(liner.TabPrints)

	for {
		var prompt string
		if isTerminal() {
			if db == nil {
				prompt = "tip> "
			} else {
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
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			break
		}

		trimmedInput := strings.TrimSpace(input)

		// Check if it's a system command
		if strings.HasPrefix(trimmedInput, ".") {
			if err := handleCmd(&db, trimmedInput, os.Stdout); err != nil {
				log.Println(err)
			}
			line.AppendHistory(trimmedInput)
			continue
		}

		// Check if database connection is established
		if db == nil {
			log.Println("Error: Not connected to any database. Use .connect to establish a connection.")
			continue
		}

		queryBuilder += input + "\n"

		// Check if input is from a pipe and ends with a semicolon
		if !isTerminal() && (len(trimmedInput) == 0 || trimmedInput[len(trimmedInput)-1] != ';') {
			log.Println("Error: Input from pipe must end with a semicolon.")
			queryBuilder = "" // Reset the query builder
			continue
		}

		// Check if the trimmed input ends with a semicolon
		if len(trimmedInput) > 0 && trimmedInput[len(trimmedInput)-1] == ';' {
			startTime := time.Now() // Start timing the query execution
			queryBuilder = strings.TrimSpace(queryBuilder)
			line.AppendHistory(queryBuilder)
			isQ, output, hasRows, affectedRows, err := executeSQL(db, queryBuilder, nil)
			if err != nil {
				log.Println(err)
				queryBuilder = "" // Reset the query builder
				continue
			}
			execTime := time.Since(startTime)
			printResults(isQ, output, *outputFormat, hasRows, execTime, affectedRows)
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
		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.Render()
	} else if outputFormat == CSV {
		if len(output) == 0 {
			if !isQ {
				fmt.Printf("status,affected_rows\nOK,%d\n", affectedRows)
			} else {
				fmt.Println("(empty result)")
			}
			goto I
		}
		cols := output[0].colNames
		fmt.Println(strings.Join(cols, ","))
		for _, row := range output {
			rowData := make([]string, len(cols))
			for i := range cols {
				val := row.colValues[i]
				rowData[i] = formatCSVValue(val)
			}
			fmt.Println(strings.Join(rowData, ","))
		}
	} else {
		log.Fatal("Invalid output format: " + outputFormat.String())
	}
I:
	if showExecDetails {
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

func greeting(db *sql.DB) {
	var clientInfo string
	if info, ok := debug.ReadBuildInfo(); ok {
		clientInfo = fmt.Sprintf("tip version: %s", info.Main.Version)
	}
	log.Println(clientInfo)

	var info string
	err := db.QueryRow("SELECT tidb_version()").Scan(&info)
	if err != nil {
		log.Printf("Failed to get server info: %v", err)
		return
	}
	log.Println("------ server info ------")
	for _, line := range strings.Split(info, "\n") {
		log.Println(line)
	}
	log.Println("-------------------------")
}

func connectWithRetry(dsn string, host string, useTLS bool) (*sql.DB, error) {
	var db *sql.DB
	var err error

	log.Printf("Connecting to TiDB at: %s...", host)

	if useTLS {
		mysql.RegisterTLSConfig("tidb", &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: host,
		})
		dsn += "&tls=tidb"
	}

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Println("Failed!")
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		log.Println("Failed!")
		return nil, err
	}

	log.Println("Connected!")
	return db, nil
}

// ConnInfo represents the connection information for a database
type ConnInfo struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// connectToDatabase attempts to connect to the database using the provided ConnInfo
func connectToDatabase(info ConnInfo) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4",
		info.User, info.Password, info.Host, info.Port, info.Database)

	// Try connecting with TLS
	db, err := connectWithRetry(dsn, info.Host, true)
	if err != nil {
		fmt.Println("Attempting connection without TLS...")
		// Try connecting without TLS
		db, err = connectWithRetry(dsn, info.Host, false)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to TiDB: %v", err)
		}
	}

	if db != nil {
		db.SetMaxOpenConns(100)
		db.SetMaxIdleConns(100)

		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping TiDB: %v", err)
		}
	}

	return db, nil
}

func main() {
	// Command-line flags
	host := flag.String("host", "", "TiDB Serverless hostname")
	port := flag.String("port", "", "TiDB port")
	user := flag.String("u", "", "TiDB username")
	dbName := flag.String("d", "", "TiDB database")
	configFile := flag.String("c", getDefaultConfigFilePath(), "Path to configuration file")
	outputFormat := flag.String("o", "table", "Output format: plain, table(default) or json")
	execSQL := flag.String("e", "", "Execute SQL statement and exit")
	version := flag.Bool("version", false, "Display version information")
	verbose := flag.Bool("v", false, "Display execution details")
	outputFile := flag.String("O", "", "Output file for results")

	// Add a flag to check if -p was explicitly set
	var passSet bool
	var pass string
	flag.Func("p", "TiDB password", func(s string) error {
		passSet = true
		pass = s
		return nil
	})

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
		if !passSet && config["password"] != "" {
			pass = config["password"]
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
	if !passSet && pass == "" {
		pass = envPass
	}
	if *dbName == "" {
		*dbName = defaultDatabase
	}

	// Create ConnInfo struct
	connInfo := ConnInfo{
		Host:     *host,
		Port:     *port,
		User:     *user,
		Password: pass,
		Database: *dbName,
	}

	// Connect to the database
	db, err := connectToDatabase(connInfo)
	if err != nil {
		log.Println("Failed to connect to TiDB:", err)
		// Continue with db as nil
	}
	if db != nil {
		defer db.Close()
		greeting(db) // Call greeting after successful connection
	}

	var resultIOWriter ResultIOWriter
	if *outputFile != "" {
		file, err := os.Create(*outputFile)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer file.Close()

		bufferedWriter := bufio.NewWriter(file)
		switch parseOutputFormat(*outputFormat) {
		case CSV:
			resultIOWriter = NewCSVResultIOWriter(bufferedWriter)
		case Plain:
			resultIOWriter = NewPlainResultIOWriter(bufferedWriter)
		case JSON:
			resultIOWriter = NewJSONResultIOWriter(bufferedWriter)
		}
	}

	// Check if -e flag is provided
	if *execSQL != "" {
		startTime := time.Now() // Start timing the query execution
		isQ, output, hasRows, affectedRows, err := executeSQL(db, *execSQL, resultIOWriter)
		if err != nil {
			log.Fatalf("Failed to execute SQL: %v", err)
		}

		if resultIOWriter != nil {
			resultIOWriter.Flush()
		} else {
			execTime := time.Since(startTime)
			printResults(isQ, output, parseOutputFormat(*outputFormat), hasRows, execTime, affectedRows)
		}

		return
	}
	if *version {
		if info, ok := debug.ReadBuildInfo(); ok && Version == "dev" {
			Version = info.Main.Version
		}
		fmt.Printf("tip version: %s\n", Version)
		os.Exit(0)
	}

	// Initialize the global output format
	initialOutputFormat := parseOutputFormat(*outputFormat)
	globalOutputFormat = &initialOutputFormat

	// Modify the repl function call to use the global output format
	repl(db, globalOutputFormat)
}
