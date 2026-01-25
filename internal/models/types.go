package models

import "time"

// OAuth token stored in .token.json
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// Metadata for Cloud Code API requests
type CloudCodeMetadata struct {
	IdeType    string `json:"ideType"`
	Platform   string `json:"platform"`
	PluginType string `json:"pluginType"`
}

// Request for loadCodeAssist API
type LoadCodeAssistRequest struct {
	Metadata CloudCodeMetadata `json:"metadata"`
}

// Response from loadCodeAssist API
type LoadCodeAssistResponse struct {
	CurrentTier             *TierInfo   `json:"currentTier,omitempty"`
	PaidTier                *TierInfo   `json:"paidTier,omitempty"`
	AllowedTiers            []TierInfo  `json:"allowedTiers,omitempty"`
	CloudAICompanionProject interface{} `json:"cloudaicompanionProject,omitempty"`
}

type TierInfo struct {
	ID        string `json:"id,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

// Request for fetchAvailableModels API
type FetchModelsRequest struct {
	Project string `json:"project,omitempty"`
}

// Response from fetchAvailableModels API
type FetchModelsResponse struct {
	Models map[string]ModelInfo `json:"models"`
}

type ModelInfo struct {
	DisplayName string    `json:"displayName"`
	Model       string    `json:"model"`
	QuotaInfo   QuotaInfo `json:"quotaInfo"`
}

type QuotaInfo struct {
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"` // ISO 8601 timestamp
}

// Parsed quota data for display
type QuotaStatus struct {
	Email  string       `json:"email,omitempty"`
	Models []ModelQuota `json:"models"`
}

type ModelQuota struct {
	Name         string    `json:"name"`
	ModelID      string    `json:"model_id"`
	RemainingPct float64   `json:"remaining_pct"`
	ResetTime    time.Time `json:"reset_time"`
	ResetIn      string    `json:"reset_in,omitempty"` // Human-readable duration
}

// AccountCredential holds an account's email and access token
type AccountCredential struct {
	Email       string `json:"email"`
	AccessToken string `json:"access_token"`
}
