package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"

	"webservice/config"
)

// ResilientHTTPClient provides HTTP client with retry logic and exponential backoff
type ResilientHTTPClient struct {
	client        *http.Client
	retryAttempts int
	baseDelay     time.Duration
}

// NewResilientHTTPClient creates a new resilient HTTP client using config
func NewResilientHTTPClient() *ResilientHTTPClient {
	cfg := config.GetConfig()
	return &ResilientHTTPClient{
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
		retryAttempts: cfg.RetryAttempts,
		baseDelay:     time.Duration(cfg.RetryBaseDelay) * time.Millisecond,
	}
}

// NewResilientHTTPClientWithConfig creates a new resilient HTTP client with custom config
func NewResilientHTTPClientWithConfig(timeout, retryAttempts int, baseDelayMs int) *ResilientHTTPClient {
	return &ResilientHTTPClient{
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		retryAttempts: retryAttempts,
		baseDelay:     time.Duration(baseDelayMs) * time.Millisecond,
	}
}

// PostJSON sends a POST request with JSON body and returns the response
func (c *ResilientHTTPClient) PostJSON(url string, body interface{}) (*http.Response, error) {
	requestBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryAttempts; attempt++ {
		if attempt > 0 {
			// Exponential backoff: baseDelay * 2^(attempt-1)
			delay := c.baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			log.Printf("Retry attempt %d/%d after %v delay", attempt, c.retryAttempts, delay)
			time.Sleep(delay)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("Request to %s failed (attempt %d): %v", url, attempt+1, err)
			continue
		}

		// Success or non-retryable error
		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Server error - close body and retry
		resp.Body.Close()
		lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
		log.Printf("Request to %s returned %d (attempt %d)", url, resp.StatusCode, attempt+1)
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", c.retryAttempts+1, lastErr)
}

// PostJSONAndDecode sends a POST request and decodes the JSON response
func (c *ResilientHTTPClient) PostJSONAndDecode(url string, body interface{}, result interface{}) error {
	resp, err := c.PostJSON(url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Get sends a GET request with retry logic
func (c *ResilientHTTPClient) Get(url string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retryAttempts; attempt++ {
		if attempt > 0 {
			delay := c.baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			log.Printf("Retry attempt %d/%d after %v delay", attempt, c.retryAttempts, delay)
			time.Sleep(delay)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("GET request to %s failed (attempt %d): %v", url, attempt+1, err)
			continue
		}

		if resp.StatusCode < 500 {
			return resp, nil
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
		log.Printf("GET request to %s returned %d (attempt %d)", url, resp.StatusCode, attempt+1)
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", c.retryAttempts+1, lastErr)
}
