package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"air-automation/internal/action"
	"air-automation/internal/site"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o-mini"
)

type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func (l LLM) Plan(ctx context.Context, req Request) (action.Plan, error) {
	task := req.Task
	if strings.TrimSpace(task) == "" {
		return action.Plan{}, fmt.Errorf("task is required")
	}
	if l.APIKey == "" {
		return action.Plan{}, fmt.Errorf("missing API key")
	}
	baseURL := strings.TrimRight(l.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := l.Model
	if model == "" {
		model = DefaultModel
	}
	client := l.Client
	if client == nil {
		client = http.DefaultClient
	}

	payload := chatRequest{
		Model:       model,
		Temperature: 0,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt(req.Site)},
			{Role: "user", Content: task},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return action.Plan{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return action.Plan{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+l.APIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return action.Plan{}, fmt.Errorf("planner request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return action.Plan{}, fmt.Errorf("planner returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return action.Plan{}, fmt.Errorf("decode planner response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return action.Plan{}, fmt.Errorf("planner returned no choices")
	}
	content := extractJSON(decoded.Choices[0].Message.Content)
	var plan action.Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return action.Plan{}, fmt.Errorf("parse planner plan: %w", err)
	}
	if plan.Task == "" {
		plan.Task = task
	}
	return plan, nil
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		return strings.TrimSpace(content)
	}
	return content
}

func systemPrompt(profile *site.Profile) string {
	prompt := "You are a browser-automation planner. Convert the user's task into a JSON object " +
		"with fields \"task\" (string) and \"steps\" (array). Each step is an object with \"kind\" " +
		"and relevant fields. Allowed kinds and fields:\n" +
		"- navigate: {url}\n" +
		"- click: {selector OR menu, consequential}\n" +
		"- fill: {selector OR field, value}\n" +
		"- read: {selector}\n" +
		"- wait: {selector} or {duration_ms}\n" +
		"- press: {selector, key}\n" +
		"- screenshot: {path}\n" +
		"- await_user: {prompt} (pauses for the human; use after a step that may trigger 2FA or CAPTCHA)\n" +
		"Use CSS selectors. For any credential or secret value use the placeholder " +
		"{{ENV:NAME}} (for example {{ENV:SITE_PASSWORD}}) and never invent a real value. " +
		"Set \"consequential\": true on steps that submit, delete, purchase, or otherwise change " +
		"important data. Respond with JSON only."
	if profile != nil {
		prompt += "\n\nActive site: " + profile.Name
		if profile.LoginURL != "" {
			prompt += "\nLogin URL: " + profile.LoginURL
		}
		if fields := profile.FieldNames(); len(fields) > 0 {
			prompt += "\nField names available via the fill \"field\" key: " + strings.Join(fields, ", ")
		}
		if menus := profile.MenuNames(); len(menus) > 0 {
			prompt += "\nMenu names available via the click \"menu\" key: " + strings.Join(menus, ", ")
		}
	}
	return prompt
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
