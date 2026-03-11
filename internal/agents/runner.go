package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var httpClient = &http.Client{Timeout: 120 * time.Second}

// callAPI sends a prompt to the Anthropic Messages API and returns the text response + usage.
func callAPI(ctx context.Context, model string, maxTokens int, systemPrompt, userPrompt string) (string, TokenUsage, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", TokenUsage{}, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	reqBody := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: userPrompt}},
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", TokenUsage{}, ctx.Err()
			case <-time.After(time.Duration(attempt*2) * time.Second):
			}
		}

		text, usage, err := doAPICall(ctx, apiKey, reqBody)
		if err == nil {
			return text, usage, nil
		}
		lastErr = err

		// Only retry on transient errors
		if strings.Contains(err.Error(), "529") || strings.Contains(err.Error(), "overloaded") ||
			strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), "connection") {
			continue
		}
		return "", TokenUsage{}, err // non-retryable
	}
	return "", TokenUsage{}, lastErr
}

func doAPICall(ctx context.Context, apiKey string, reqBody anthropicRequest) (string, TokenUsage, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", TokenUsage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(data))
	if err != nil {
		return "", TokenUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", TokenUsage{}, err
	}

	var ar anthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return "", TokenUsage{}, fmt.Errorf("parsing API response: %w", err)
	}

	if ar.Error != nil {
		return "", TokenUsage{}, fmt.Errorf("API error (%s): %s", ar.Error.Type, ar.Error.Message)
	}
	if resp.StatusCode != 200 {
		return "", TokenUsage{}, fmt.Errorf("API HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}
	if len(ar.Content) == 0 {
		return "", TokenUsage{}, fmt.Errorf("API returned empty content")
	}

	usage := TokenUsage{
		InputTokens:  ar.Usage.InputTokens,
		OutputTokens: ar.Usage.OutputTokens,
		Model:        reqBody.Model,
	}
	return ar.Content[0].Text, usage, nil
}

// parseJSONResponse robustly extracts a JSON array or object from LLM output.
// LLMs sometimes wrap JSON in markdown fences or add preamble text.
func parseJSONResponse(raw string, target any) error {
	raw = strings.TrimSpace(raw)

	// Strip markdown code fences
	if idx := strings.Index(raw, "```json"); idx >= 0 {
		raw = raw[idx+7:]
		if end := strings.Index(raw, "```"); end >= 0 {
			raw = raw[:end]
		}
	} else if idx := strings.Index(raw, "```"); idx >= 0 {
		raw = raw[idx+3:]
		if end := strings.Index(raw, "```"); end >= 0 {
			raw = raw[:end]
		}
	}
	raw = strings.TrimSpace(raw)

	// Try direct parse
	if err := json.Unmarshal([]byte(raw), target); err == nil {
		return nil
	}

	// Extract first JSON array or object
	start := strings.IndexAny(raw, "[{")
	if start < 0 {
		return fmt.Errorf("no JSON found in response")
	}
	open := raw[start]
	close := byte(']')
	if open == '{' {
		close = '}'
	}
	end := strings.LastIndexByte(raw, close)
	if end < start {
		return fmt.Errorf("malformed JSON in response")
	}
	return json.Unmarshal([]byte(raw[start:end+1]), target)
}

// CallAPIForConsolidation is a public wrapper used by the memory consolidation agent.
func CallAPIForConsolidation(ctx context.Context, model string, maxTokens int, system, user string) (string, TokenUsage, error) {
	return callAPI(ctx, model, maxTokens, system, user)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
