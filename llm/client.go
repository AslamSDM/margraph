package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"margraf/logger"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	ApiKey   string
	Model    string
	Provider string // "gemini" or "openrouter"
	BaseURL  string

	// Circuit Breaker State
	failureCount    int
	lastFailureTime time.Time
	circuitOpen     bool

	// Rate Limiting
	requestCount    int
	windowStart     time.Time
	maxRequestsPerMinute int

	// Fallback Client
	fallback *Client
}

func NewClient() *Client {
	var primary *Client
	var fallback *Client

	// 1. Primary: OpenRouter with Grok-4.1-fast
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		model := os.Getenv("OPENROUTER_MODEL")
		if model == "" {
			model = "x-ai/grok-beta" // Grok-4.1-fast free tier
		}
		logger.Info(logger.StatusOK, "Primary LLM: OpenRouter (%s)", model)
		primary = &Client{
			ApiKey:               key,
			Model:                model,
			Provider:             "openrouter",
			BaseURL:              "https://openrouter.ai/api/v1/chat/completions",
			maxRequestsPerMinute: 60,
			windowStart:          time.Now(),
		}
	}

	// 2. Fallback: Gemini Direct
	if geminiKey := os.Getenv("GEMINI_API_KEY"); geminiKey != "" {
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-1.5-flash"
		}
		logger.Info(logger.StatusOK, "Fallback LLM: Google Gemini (%s)", model)
		fallback = &Client{
			ApiKey:               geminiKey,
			Model:                model,
			Provider:             "gemini",
			BaseURL:              "https://generativelanguage.googleapis.com/v1beta/models",
			maxRequestsPerMinute: 60,
			windowStart:          time.Now(),
		}
	}

	// Link fallback to primary
	if primary != nil {
		primary.fallback = fallback
		return primary
	}

	// If no OpenRouter key, use Gemini as primary
	if fallback != nil {
		return fallback
	}

	// No API keys configured
	logger.Error(logger.StatusErr, "No API keys configured (OPENROUTER_API_KEY or GEMINI_API_KEY)")
	return &Client{
		ApiKey: "",
		Model:  "",
		Provider: "",
	}
}

// --- Gemini Types ---
type Part struct {
	Text string `json:"text"`
}
type Content struct {
	Parts []Part `json:"parts"`
}
type GenerateRequest struct {
	Contents []Content `json:"contents"`
}
type GenerateResponse struct {
	Candidates []struct {
		Content Content `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Details []ErrorDetail   `json:"details"`
	} `json:"error"`
}
type ErrorDetail struct {
	Type       string `json:"@type"`
	RetryDelay string `json:"retryDelay"`
}

// --- OpenRouter / OpenAI Types ---
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}
type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    interface{} `json:"code"` // Can be int or string
	} `json:"error"`
}

// checkCircuitBreaker determines if the circuit is open (too many failures)
func (c *Client) checkCircuitBreaker() error {
	const maxFailures = 5
	const cooldownPeriod = 60 * time.Second

	if c.circuitOpen {
		// Check if cooldown period has passed
		if time.Since(c.lastFailureTime) > cooldownPeriod {
			logger.InfoDepth(1, logger.StatusRec, "Circuit breaker cooling down, attempting reset...")
			c.circuitOpen = false
			c.failureCount = 0
		} else {
			return fmt.Errorf("circuit breaker OPEN - too many API failures. Retry after %v", cooldownPeriod-time.Since(c.lastFailureTime))
		}
	}

	return nil
}

// recordFailure increments failure count and potentially opens circuit
func (c *Client) recordFailure() {
	c.failureCount++
	c.lastFailureTime = time.Now()

	if c.failureCount >= 5 {
		c.circuitOpen = true
		logger.WarnDepth(1, logger.StatusWarn, "CIRCUIT BREAKER OPENED after %d consecutive failures", c.failureCount)
	}
}

// recordSuccess resets failure counter
func (c *Client) recordSuccess() {
	if c.failureCount > 0 {
		logger.InfoDepth(1, logger.StatusOK, "API call succeeded, resetting failure count")
	}
	c.failureCount = 0
	c.circuitOpen = false
}

// enforceRateLimit checks and enforces request rate limiting
func (c *Client) enforceRateLimit() error {
	now := time.Now()

	// Reset window if needed
	if now.Sub(c.windowStart) > time.Minute {
		c.windowStart = now
		c.requestCount = 0
	}

	// Check if we've exceeded rate limit
	if c.requestCount >= c.maxRequestsPerMinute {
		waitTime := time.Minute - now.Sub(c.windowStart)
		return fmt.Errorf("rate limit exceeded (%d requests/min). Wait %v", c.maxRequestsPerMinute, waitTime)
	}

	c.requestCount++
	return nil
}

func (c *Client) Complete(prompt string) (string, error) {
	if c.ApiKey == "" {
		return "", errors.New("API_KEY not set (OPENROUTER_API_KEY or GEMINI_API_KEY)")
	}

	// Check circuit breaker
	if err := c.checkCircuitBreaker(); err != nil {
		// If circuit is open and we have a fallback, try fallback
		if c.fallback != nil {
			logger.Warn(logger.StatusWarn, "Primary LLM circuit open, using fallback (%s)", c.fallback.Provider)
			return c.fallback.Complete(prompt)
		}
		return "", err
	}

	// Enforce rate limiting
	if err := c.enforceRateLimit(); err != nil {
		// If rate limited and we have a fallback, try fallback
		if c.fallback != nil {
			logger.Warn(logger.StatusWarn, "Primary LLM rate limited, using fallback (%s)", c.fallback.Provider)
			return c.fallback.Complete(prompt)
		}
		return "", err
	}

	var result string
	var err error

	if c.Provider == "openrouter" {
		result, err = c.completeOpenRouter(prompt)
	} else {
		result, err = c.completeGemini(prompt)
	}

	// Update circuit breaker state
	if err != nil {
		c.recordFailure()

		// If primary failed and we have a fallback, try fallback
		if c.fallback != nil {
			logger.Warn(logger.StatusWarn, "Primary LLM failed (%v), trying fallback (%s)", err, c.fallback.Provider)
			return c.fallback.Complete(prompt)
		}
	} else {
		c.recordSuccess()
	}

	return result, err
}

func (c *Client) completeOpenRouter(prompt string) (string, error) {
	reqBody := ChatRequest{
		Model: c.Model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}
	jsonData, _ := json.Marshal(reqBody)

	// Simple retry loop for OpenRouter too
	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, _ := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
		req.Header.Set("Authorization", "Bearer "+c.ApiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://margraf.app") // Required by OpenRouter
		req.Header.Set("X-Title", "Margraf FDKG")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == 200 {
			var chatResp ChatResponse
			if err := json.Unmarshal(body, &chatResp); err != nil {
				return "", err
			}
			if len(chatResp.Choices) > 0 {
				return chatResp.Choices[0].Message.Content, nil
			}
			return "", errors.New("no content in OpenRouter response")
		}

		if resp.StatusCode == 429 {
			logger.InfoDepth(2, logger.StatusWait, "OpenRouter Rate Limit. Retrying in 5s...")
			time.Sleep(5 * time.Second)
			continue
		}
		
		return "", fmt.Errorf("OpenRouter error %d: %s", resp.StatusCode, string(body))
	}
	return "", errors.New("max retries exceeded")
}

func (c *Client) completeGemini(prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", c.BaseURL, c.Model, c.ApiKey)

	reqBody := GenerateRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	maxRetries := 5
	var body []byte
	var resp *http.Response

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		body, _ = io.ReadAll(resp.Body)

		if resp.StatusCode == 200 {
			break
		}

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			if attempt == maxRetries {
				break 
			}

			delay := time.Duration(5*(1<<attempt)) * time.Second 

			var apiErr struct {
				Error struct {
					Details []ErrorDetail `json:"details"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &apiErr) == nil {
				for _, detail := range apiErr.Error.Details {
					if strings.Contains(detail.Type, "RetryInfo") && detail.RetryDelay != "" {
						if d, err := time.ParseDuration(detail.RetryDelay); err == nil {
							delay = d + 500*time.Millisecond
						}
					}
				}
			}

			logger.InfoDepth(2, logger.StatusWait, "Rate limit (%d). Retrying in %v...", resp.StatusCode, delay)
			time.Sleep(delay)
			continue
		}

		msg := fmt.Sprintf("API request failed with status %d: %s", resp.StatusCode, string(body))
		if resp.StatusCode == 404 {
			msg += fmt.Sprintf("\n[Hint] Model '%s' not found.", c.Model)
		}
		return "", errors.New(msg)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API request failed after retries with status %d: %s", resp.StatusCode, string(body))
	}

	var genResp GenerateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return "", err
	}

	if genResp.Error != nil {
		return "", fmt.Errorf("API error: %s", genResp.Error.Message)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("no content generated")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}
