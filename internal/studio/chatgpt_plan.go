package studio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI OSS ChatGPT plan access requires store:false, stream:true and input arrays.
// The OAuth access token is supplied by the local application's verified sign-in.
func askChatGPTPlan(ctx context.Context, token, model, prompt string) (string, error) {
	payload := map[string]any{"model": model, "input": []map[string]string{{"role": "user", "content": prompt}}, "store": false, "stream": true}
	b, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/responses", bytes.NewReader(b))
	if e != nil {
		return "", e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Timeout: 105 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2500))
		return "", fmt.Errorf("ChatGPT plan request HTTP %d: %s", resp.StatusCode, string(b))
	}
	return parseChatGPTStream(resp.Body)
}
func parseChatGPTStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 3*1024*1024)
	var out strings.Builder
	completed := false
	var event string
	var chunks []string
	flush := func() error {
		if len(chunks) == 0 {
			event = ""
			return nil
		}
		raw := strings.Join(chunks, "\n")
		chunks = nil
		if raw == "[DONE]" {
			event = ""
			return nil
		}
		var obj struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(raw), &obj) != nil {
			event = ""
			return nil
		}
		kind := obj.Type
		if kind == "" {
			kind = event
		}
		event = ""
		switch kind {
		case "response.output_text.delta":
			out.WriteString(obj.Delta)
		case "response.completed":
			completed = true
		case "response.failed", "response.incomplete":
			return fmt.Errorf("ChatGPT plan inference %s: %s %s", kind, obj.Response.Error.Code, obj.Response.Error.Message)
		case "error":
			return fmt.Errorf("ChatGPT plan error: %s %s", obj.Error.Code, obj.Error.Message)
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if e := flush(); e != nil {
				return "", e
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			chunks = append(chunks, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if e := scanner.Err(); e != nil {
		return "", e
	}
	if e := flush(); e != nil {
		return "", e
	}
	if !completed {
		return "", errors.New("ChatGPT response stream ended before response.completed")
	}
	if strings.TrimSpace(out.String()) == "" {
		return "", errors.New("ChatGPT response completed without text")
	}
	return out.String(), nil
}
