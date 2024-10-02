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
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/olekukonko/tablewriter"
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
	file, err := os.Open(configPath)
	if err != nil {
		return config, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		var key, value string
		fmt.Sscanf(line, "%s %s", &key, &value)
		config[key] = value
	}
	return config, scanner.Err()
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

func repl(db *sql.DB, outputFormat OutputFormat) {
	scanner := bufio.NewScanner(os.Stdin)
	_, err := db.Exec("SET GLOBAL tidb_multi_statement_mode='ON';")
	if err != nil {
		log.Printf("Failed to set tidb_multi_statement_mode: %v", err)
		os.Exit(1)
	}
	for {
		if isTerminal() {
			fmt.Printf("> ")
		}
		exit := scanner.Scan()
		if !exit {
			break
		}
		query := scanner.Text()
		if query == "" {
			continue
		}
		rows, err := db.Query(query)
		if err != nil {
			log.Printf("Failed to execute SQL: %v", err)
			continue
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			log.Printf("Failed to get column info: %v", err)
			continue
		}

		results := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))

		for i := range results {
			pointers[i] = &results[i]
		}

		var output []RowResult
		hasRows := false
		for rows.Next() {
			hasRows = true
			err := rows.Scan(pointers...)
			if err != nil {
				log.Printf("Failed to read data: %v", err)
				continue
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
		printResults(output, outputFormat, hasRows)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Failed to read input: %v", err)
	}
}

func printResults(output []RowResult, outputFormat OutputFormat, hasRows bool) {
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
				switch v := val.(type) {
				case nil:
					fmt.Printf("%s: NULL ", col)
				case bool:
					fmt.Printf("%s: %t ", col, v)
				case int, int64:
					fmt.Printf("%s: %d ", col, v)
				case float64:
					fmt.Printf("%s: %f ", col, v)
				case string:
					fmt.Printf("%s: %s ", col, v)
				case []byte:
					fmt.Printf("%s: %s ", col, string(v))
				case time.Time:
					fmt.Printf("%s: %s ", col, v.Format("2006-01-02 15:04:05"))
				default:
					fmt.Printf("%s: %v ", col, v)
				}
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
				switch v := val.(type) {
				case nil:
					rowData[i] = ""
				case bool:
					rowData[i] = fmt.Sprintf("%t", v)
				case int, int64:
					rowData[i] = fmt.Sprintf("%d", v)
				case float64:
					rowData[i] = fmt.Sprintf("%f", v)
				case string:
					rowData[i] = v
				case []byte:
					rowData[i] = string(v)
				case time.Time:
					rowData[i] = v.Format("2006-01-02 15:04:05")
				default:
					rowData[i] = fmt.Sprintf("%v", v)
				}
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
}

func main() {
	// Command-line flags
	host := flag.String("h", "127.0.0.1", "TiDB Serverless hostname")
	port := flag.String("p", "4000", "TiDB port")
	user := flag.String("u", "root", "TiDB username")
	pass := flag.String("P", "", "TiDB password")
	dbName := flag.String("d", "test", "TiDB database")
	configFile := flag.String("c", "", "Path to configuration file")
	outputFormat := flag.String("o", "plain", "Output format: plain or json")
	flag.Parse()

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

	// Execute queries
	repl(db, parseOutputFormat(*outputFormat))
}
