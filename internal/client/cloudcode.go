package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"quota-ag/internal/models"
)

const (
	baseURL    = "https://cloudcode-pa.googleapis.com"
	userAgent  = "antigravity"
	maxRetries = 3
)

// Shared HTTP transport for connection pooling across clients
var sharedTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     90 * time.Second,
}

// CloudCodeClient handles API calls to Google Cloud Code
type CloudCodeClient struct {
	httpClient  *http.Client
	accessToken string
	projectID   string
}

// NewCloudCodeClient creates a new Cloud Code API client
func NewCloudCodeClient(accessToken string) *CloudCodeClient {
	return &CloudCodeClient{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: sharedTransport,
		},
		accessToken: accessToken,
	}
}

// FetchQuota fetches the quota information for all models
func (c *CloudCodeClient) FetchQuota(ctx context.Context) (*models.QuotaStatus, error) {
	// First, get the project ID
	if err := c.loadProjectInfo(ctx); err != nil {
		return nil, fmt.Errorf("failed to load project info: %w", err)
	}

	// Then fetch available models with quota
	modelsResp, err := c.fetchAvailableModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	// Convert to QuotaStatus
	status := &models.QuotaStatus{
		Models: make([]models.ModelQuota, 0, len(modelsResp.Models)),
	}

	for _, model := range modelsResp.Models {
		resetTime, _ := time.Parse(time.RFC3339, model.QuotaInfo.ResetTime)
		resetIn := formatDuration(time.Until(resetTime))

		status.Models = append(status.Models, models.ModelQuota{
			Name:         model.DisplayName,
			ModelID:      model.Model,
			RemainingPct: model.QuotaInfo.RemainingFraction * 100,
			ResetTime:    resetTime,
			ResetIn:      resetIn,
		})
	}

	return status, nil
}

// loadProjectInfo calls loadCodeAssist to get the project ID
func (c *CloudCodeClient) loadProjectInfo(ctx context.Context) error {
	reqBody := models.LoadCodeAssistRequest{
		Metadata: models.CloudCodeMetadata{
			IdeType:    "ANTIGRAVITY",
			Platform:   "PLATFORM_UNSPECIFIED",
			PluginType: "GEMINI",
		},
	}

	var resp models.LoadCodeAssistResponse
	if err := c.doRequest(ctx, "/v1internal:loadCodeAssist", reqBody, &resp); err != nil {
		return err
	}

	// Extract project ID from response
	c.projectID = extractProjectID(resp.CloudAICompanionProject)
	return nil
}

// extractProjectID extracts the project ID from the cloudaicompanionProject field
func extractProjectID(project interface{}) string {
	if project == nil {
		return ""
	}

	// If it's a string, return directly
	if s, ok := project.(string); ok {
		return s
	}

	// If it's a map with an "id" field
	if m, ok := project.(map[string]interface{}); ok {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}

	return ""
}

// fetchAvailableModels calls fetchAvailableModels API
func (c *CloudCodeClient) fetchAvailableModels(ctx context.Context) (*models.FetchModelsResponse, error) {
	reqBody := models.FetchModelsRequest{
		Project: c.projectID,
	}

	var resp models.FetchModelsResponse
	if err := c.doRequest(ctx, "/v1internal:fetchAvailableModels", reqBody, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// doRequest performs an HTTP request with retries
func (c *CloudCodeClient) doRequest(ctx context.Context, path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter to prevent thundering herd
			backoff := time.Duration(1<<attempt) * time.Second
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			time.Sleep(backoff + jitter)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close() // Close immediately, not deferred (avoid resource leak in loop)
		if err != nil {
			lastErr = err
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("failed to unmarshal response: %w", err)
			}
			return nil
		case http.StatusUnauthorized:
			return fmt.Errorf("authentication expired, please re-authenticate with --reauth")
		case http.StatusForbidden:
			return fmt.Errorf("access forbidden: %s", string(respBody))
		case http.StatusTooManyRequests, http.StatusInternalServerError,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
			continue
		default:
			return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	return fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}

// formatDuration formats a duration as "Xh Ym" or "Xm Ys"
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}

	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	return fmt.Sprintf("%ds", seconds)
}
