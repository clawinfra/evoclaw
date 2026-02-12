package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

type ChatRequest struct {
	Agent   string `json:"agent"`
	Message string `json:"message"`
	From    string `json:"from"`
}

type ChatResponse struct {
	Agent     string `json:"agent"`
	Message   string `json:"message"`
	Model     string `json:"model"`
	Timestamp string `json:"timestamp"`
}

type Agent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func main() {
	apiURL := flag.String("api", "http://localhost:8421", "EvoClaw API URL")
	agentID := flag.String("agent", "", "Agent ID to chat with")
	flag.Parse()

	fmt.Printf("%s╭──────────────────────────────────────╮%s\n", colorBlue, colorReset)
	fmt.Printf("%s│     🧬 EvoClaw Terminal (TUI)       │%s\n", colorBlue, colorReset)
	fmt.Printf("%s╰──────────────────────────────────────╯%s\n", colorBlue, colorReset)
	fmt.Println()

	// Load agents
	agents, err := loadAgents(*apiURL)
	if err != nil {
		fmt.Printf("%s✗ Failed to load agents: %v%s\n", colorYellow, err, colorReset)
		os.Exit(1)
	}

	if len(agents) == 0 {
		fmt.Printf("%s✗ No agents available%s\n", colorYellow, colorReset)
		os.Exit(1)
	}

	// Select agent
	var selectedAgent string
	if *agentID != "" {
		selectedAgent = *agentID
	} else if len(agents) == 1 {
		selectedAgent = agents[0].ID
	} else {
		fmt.Println("Available agents:")
		for i, a := range agents {
			fmt.Printf("  %d. %s%s%s (%s%s%s)\n",
				i+1,
				colorGreen, a.ID, colorReset,
				colorGray, a.Status, colorReset,
			)
		}
		fmt.Print("\nSelect agent (1-", len(agents), "): ")
		var choice int
		fmt.Scanln(&choice)
		if choice < 1 || choice > len(agents) {
			fmt.Println("Invalid choice")
			os.Exit(1)
		}
		selectedAgent = agents[choice-1].ID
	}

	fmt.Printf("%s✓ Connected to %s%s%s\n", colorGreen, colorBlue, selectedAgent, colorReset)
	fmt.Printf("%sType 'exit' or 'quit' to exit%s\n\n", colorGray, colorReset)

	// Chat loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%syou%s > ", colorYellow, colorReset)
		if !scanner.Scan() {
			break
		}

		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}

		if message == "exit" || message == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		// Send message
		resp, err := sendMessage(*apiURL, selectedAgent, message)
		if err != nil {
			fmt.Printf("%s✗ Error: %v%s\n\n", colorYellow, err, colorReset)
			continue
		}

		// Display response
		fmt.Printf("%s%s%s > %s\n\n", colorGreen, selectedAgent, colorReset, resp.Message)
	}
}

func loadAgents(apiURL string) ([]Agent, error) {
	resp, err := http.Get(apiURL + "/api/agents")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var agents []Agent
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, err
	}

	return agents, nil
}

func sendMessage(apiURL, agent, message string) (*ChatResponse, error) {
	req := ChatRequest{
		Agent:   agent,
		Message: message,
		From:    "tui",
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", apiURL+"/api/chat", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != 200 {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	var resp ChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
