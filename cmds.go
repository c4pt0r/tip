package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
)

type SystemCmd interface {
	Handle(args []string, resultWriter io.Writer) error
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
			return cmd.Handle(params, resultWriter)
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

func (cmd HelpCmd) Handle(args []string, resultWriter io.Writer) error {
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

func (cmd VerCmd) Handle(args []string, resultWriter io.Writer) error {
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

func (cmd RefreshCmd) Handle(args []string, resultWriter io.Writer) error {
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

func (cmd ConnectCmd) Handle(args []string, resultWriter io.Writer) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: .connect <host> <port> <user> <password> [database]")
	}

	host := args[0]
	port := args[1]
	user := args[2]
	pass := args[3]
	dbName := "test"
	if len(args) > 4 {
		dbName = args[4]
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

func (cmd OutputFormatCmd) Handle(args []string, resultWriter io.Writer) error {
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

type AskCmd struct{}

func (cmd AskCmd) Name() string {
	return ".ask"
}

func (cmd AskCmd) Description() string {
	return "Ask a question to the database"
}

func (cmd AskCmd) Usage() string {
	return ".ask <question>"
}

// AskResponse struct for parsing the API response
type AskResponse struct {
	Content string `json:"content"`
}

// askQuestion sends a question to the TiDB AI API and returns the response
func askQuestion(question string) (string, error) {
	url := "https://tidb.ai/api/v1/chats"

	// Construct request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": question,
			},
		},
		"chat_engine": "default",
		"stream":      false,
	})
	if err != nil {
		return "", fmt.Errorf("error marshaling request body: %v", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	// Set request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accept", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	// Parse response
	var askResp AskResponse
	err = json.Unmarshal(body, &askResp)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	return askResp.Content, nil
}

func (cmd AskCmd) Handle(args []string, resultWriter io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: .ask <question>")
	}
	question := strings.Join(args, " ")

	// Start the loading animation in a separate goroutine
	done := make(chan bool)
	go loadingAnimation(resultWriter, done)

	answer, err := askQuestion(question)

	// Stop the loading animation
	done <- true

	if err != nil {
		return fmt.Errorf("error asking question: %v", err)
	}

	// Clear the loading animation line
	resultWriter.Write([]byte("\r\033[K"))
	resultWriter.Write([]byte(answer + "\n"))

	// Extract SQL statements
	sqlStatements := extractSQLStatements(answer)

	// If SQL statements are found, create a selection menu
	if len(sqlStatements) > 0 {
		prompt := promptui.Select{
			Label: "Select SQL statement to execute",
			Items: sqlStatements,
		}

		_, ret, err := prompt.Run()

		if err != nil {
			return fmt.Errorf("prompt")
		}

		replSuggestion = ret
	}

	return nil
}

func extractSQLStatements(text string) []string {
	re := regexp.MustCompile("(?s)```sql\\s*(.+?)\\s*```")
	matches := re.FindAllStringSubmatch(text, -1)

	statements := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			statements = append(statements, strings.TrimSpace(match[1]))
		}
	}

	return statements
}

func loadingAnimation(w io.Writer, done chan bool) {
	frames := []string{"-", "\\", "|", "/"}
	i := 0
	for {
		select {
		case <-done:
			return
		default:
			w.Write([]byte(fmt.Sprintf("\rThinking %s", frames[i])))
			time.Sleep(100 * time.Millisecond)
			i = (i + 1) % len(frames)
		}
	}
}
