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

	refinedQuestion := refineQuestion(question)
	answer, err := askQuestion(refinedQuestion)

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
			Label: "Select SQL statement to execute (Ctrl+C to cancel)",
			Items: sqlStatements,
		}
		_, ret, _ := prompt.Run()
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
			// multiple lines into one line
			stmt := strings.TrimSpace(match[1])
			// split by \n and trim space for each line, and join them with space
			lines := strings.Split(stmt, "\n")
			var trimedStmt string
			for _, line := range lines {
				trimedStmt += strings.TrimSpace(line) + " "
			}
			statements = append(statements, trimedStmt)
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

func refineQuestion(question string) string {
	template := `Based on the context, answer the question in user's language
	Context:
	%s
	Question:
	%s`
	var context string
	if globalDB != nil {
		var curDB string
		globalDB.QueryRow("SELECT DATABASE()").Scan(&curDB)
		if curDB != "" {
			tableNames, _ := getTableNames(globalDB, curDB)
			tableNameSet := make(map[string]bool)
			for _, tableName := range tableNames {
				tableNameSet[tableName] = true
			}

			re := regexp.MustCompile(`\b[a-zA-Z_]\w*\b`)
			matches := re.FindAllString(question, -1)

			foundTables := make(map[string]string)

			for _, match := range matches {
				if tableNameSet[match] {
					var createTable string
					err := globalDB.QueryRow("SHOW CREATE TABLE "+match).Scan(&match, &createTable)
					if err != nil {
						continue
					}
					foundTables[match] = createTable
				}
			}

			for tableName, createTable := range foundTables {
				context += fmt.Sprintf("`%s` schema: %s\n---\n", tableName, createTable)
			}
		}
	}
	refinedQuestion := fmt.Sprintf(template, context, question)
	return refinedQuestion
}
