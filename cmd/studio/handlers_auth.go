package main

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/boormajor/podcast-ai-studio/internal/studio"
)

// Subscription sign-in state stays inside the local Go process; the browser
// only ever sees connection status, never tokens.

func (a *application) authStatus(w http.ResponseWriter, _ *http.Request) { reply(w, a.auth.status()) }

func (a *application) authChatGPTStart(w http.ResponseWriter, _ *http.Request) {
	url, err := a.auth.start()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	reply(w, map[string]string{"url": url})
	go openBrowser(url)
}

func (a *application) authChatGPTModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := a.auth.models(ctx)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	reply(w, map[string]any{"models": models})
}

func (a *application) authChatGPTDisconnect(w http.ResponseWriter, _ *http.Request) {
	if err := a.auth.disconnect(); err != nil {
		fail(w, 500, err.Error())
		return
	}
	reply(w, map[string]bool{"ok": true})
}

func (a *application) authClaudeStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	connected, message := studio.ClaudeCodeStatus(ctx)
	reply(w, map[string]any{"connected": connected, "message": message})
}

func (a *application) authClaudeLogin(w http.ResponseWriter, _ *http.Request) {
	if !studio.HasTool("claude") {
		fail(w, 400, "Install the official Claude Code CLI first: https://code.claude.com/docs/en/setup")
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 400, "Open a terminal and run: claude auth login")
		return
	}
	// Fixed command only; no caller-supplied strings reach the shell.
	if err := exec.Command("cmd", "/c", "start", "", "cmd", "/k", "claude", "auth", "login").Start(); err != nil {
		fail(w, 500, "Unable to open the Claude login terminal: "+err.Error())
		return
	}
	reply(w, map[string]bool{"ok": true})
}

// openBrowser opens a URL in the user's default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
