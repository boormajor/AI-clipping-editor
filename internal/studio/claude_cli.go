package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Claude Code owns sign-in, credential storage, and subscription handling.
// This does not read or reuse OAuth tokens or mimic an Anthropic client.
func ClaudeCodeStatus(ctx context.Context) (bool, string) {
	if _, err := exec.LookPath("claude"); err != nil {
		return false, "Claude Code not installed. Install Claude Code and run: claude auth login"
	}
	cmd := exec.CommandContext(ctx, "claude", "auth", "status")
	cmd.Env = cleanClaudeEnv()
	b, err := cmd.CombinedOutput()
	if err != nil {
		return false, "Sign in to Claude Code: claude auth login"
	}
	var status struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}
	if json.Unmarshal(b, &status) != nil {
		return true, "Claude Code is installed; verify subscription in Claude Code"
	}
	if !status.LoggedIn {
		return false, "Claude Code is not signed in"
	}
	if status.AuthMethod == "api_key" || status.AuthMethod == "apiKey" {
		return false, "Claude Code is connected via an API key, not a subscription. Sign in with your Claude Pro account to avoid API charges."
	}
	return true, "Claude Code account signed in"
}
func cleanClaudeEnv() []string {
	// Do not let an API billing environment variable silently override subscription login.
	blocked := []string{"ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN=", "ANTHROPIC_BASE_URL=", "CLAUDE_CODE_USE_BEDROCK=", "CLAUDE_CODE_USE_VERTEX=", "CLAUDE_CODE_USE_FOUNDRY="}
	var out []string
	for _, v := range os.Environ() {
		s := strings.ToUpper(v)
		skip := false
		for _, p := range blocked {
			if strings.HasPrefix(s, p) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, v)
		}
	}
	return out
}
func askClaudeCode(ctx context.Context, prompt string) (string, error) {
	if _, e := exec.LookPath("claude"); e != nil {
		return "", errors.New("Claude Code CLI is not installed. Install from https://code.claude.com/docs/en/setup and run 'claude auth login' first")
	}
	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "json", "--no-session-persistence", "--tools", "")
	cmd.Env = cleanClaudeEnv()
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Claude Code failed: %w; %s", err, truncate(stderr.String(), 350))
	}
	var response struct {
		IsError bool     `json:"is_error"`
		Result  string   `json:"result"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return "", fmt.Errorf("Claude returned invalid JSON output: %w", err)
	}
	if response.IsError {
		return "", fmt.Errorf("Claude error: %s", truncate(strings.Join(response.Errors, "; ")+" "+response.Result, 500))
	}
	if strings.TrimSpace(response.Result) == "" {
		return "", errors.New("Claude returned an empty result")
	}
	return response.Result, nil
}
