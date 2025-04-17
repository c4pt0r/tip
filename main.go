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
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/olekukonko/tablewriter"
	"github.com/peterh/liner"
	"golang.org/x/term"
)

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
var replSuggestion string // Used by ask_cmd.go for REPL suggestion

var (
	globalDB     *sql.DB
	globalDBLock sync.RWMutex
	lastUsedDB   string // Store the last used database
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

// GetLastUsedDB returns the last used database
func GetLastUsedDB() string {
	globalDBLock.RLock()
	defer globalDBLock.RUnlock()
	return lastUsedDB
}

// SetLastUsedDB sets the last used database
func SetLastUsedDB(db string) {
	globalDBLock.Lock()
	defer globalDBLock.Unlock()
	lastUsedDB = db
}

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
				// Store the current database as the last used database
				if curDB != "(none)" {
					SetLastUsedDB(curDB)
				}
				if queryBuilder == "" {
					prompt = fmt.Sprintf("%s> ", curDB)
				} else {
					prompt = fmt.Sprintf("%s>>> ", curDB)
				}
			}
		}
		var input string
		var err error
		if replSuggestion != "" {
			// Use PromptWithSuggestion when replSuggestion is not empty
			input, err = line.PromptWithSuggestion(prompt, replSuggestion, len(replSuggestion))
		} else {
			// Use regular Prompt when replSuggestion is empty
			input, err = line.Prompt(prompt)
		}

		if err != nil {
			break
		}

		// Reset replSuggestion after each input
		replSuggestion = ""

		trimmedInput := strings.TrimSpace(input)

		// Check if it's a system command
		if strings.HasPrefix(trimmedInput, ".") {
			if err := handleCmd(trimmedInput, os.Stdout); err != nil {
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

var (
	Version         = "dev"
	showExecDetails = false
)

// ConnInfo represents the connection information for a database
type ConnInfo struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

func greeting(db *sql.DB) {
	if !isTerminal() {
		return
	}
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

// connectToDatabase attempts to connect to the database using the provided ConnInfo
func connectToDatabase(info ConnInfo) error {
	// If no database is specified and we have a last used database, use it
	if info.Database == "" && GetLastUsedDB() != "" {
		info.Database = GetLastUsedDB()
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4",
		info.User, info.Password, info.Host, info.Port, info.Database)

	// Try connecting with TLS
	db, err := connectWithRetry(dsn, info.Host, true)
	if err != nil {
		fmt.Println("Attempting connection without TLS...")
		// Try connecting without TLS
		db, err = connectWithRetry(dsn, info.Host, false)
		if err != nil {
			return fmt.Errorf("failed to connect to TiDB: %v", err)
		}
	}

	if db != nil {
		db.SetMaxOpenConns(100)
		db.SetMaxIdleConns(100)

		if err := db.Ping(); err != nil {
			return fmt.Errorf("failed to ping TiDB: %v", err)
		}
	}

	// Update global DB variable
	SetDB(db)
	return nil
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
		// get term width
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			log.Println(err)
		}
		table.SetColWidth(width)
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
		table.SetAutoFormatHeaders(false)
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
		printExecutionDetails(execTime, hasRows, output, affectedRows)
	}
}

func printExecutionDetails(execTime time.Duration, hasRows bool, output []RowResult, affectedRows int64) {
	grey := color.New(color.FgHiBlack).SprintFunc()

	fmt.Fprintf(os.Stderr, "%s\n", grey(fmt.Sprintf("Execution time: %s", execTime)))
	if hasRows {
		fmt.Fprintf(os.Stderr, "%s\n", grey(fmt.Sprintf("Rows in result: %d", len(output))))
	}
	if affectedRows > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", grey(fmt.Sprintf("Affected rows: %d", affectedRows)))
	}
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
	err := connectToDatabase(connInfo)
	if err != nil {
		log.Println("Failed to connect to TiDB:", err)
		// Continue with db as nil
	}
	if GetDB() != nil {
		defer GetDB().Close()
		greeting(GetDB()) // Call greeting after successful connection
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
		isQ, output, hasRows, affectedRows, err := executeSQL(GetDB(), *execSQL, resultIOWriter)
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
	repl(GetDB(), globalOutputFormat)
}
