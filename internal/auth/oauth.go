package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"quota-ag/internal/crypto"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// OAuth credentials from Antigravity extension
	clientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	clientSecret = "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"
	tokensDir    = ".tokens"
	callbackPort = 11451
)

var (
	scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}
)

// UserInfo contains the authenticated user's information
type UserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// AccountToken stores token with associated email
type AccountToken struct {
	Email        string    `json:"email"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// OAuthClient handles authentication with Google
type OAuthClient struct {
	config *oauth2.Config
}

// NewOAuthClient creates a new OAuth client
func NewOAuthClient() *OAuthClient {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  fmt.Sprintf("http://localhost:%d/callback", callbackPort),
	}

	client := &OAuthClient{
		config: config,
	}

	// Migrate any existing plain-text tokens
	client.migrateOldTokens()

	return client
}

// migrateOldTokens migrates old plain-text .json tokens to encrypted .enc format
func (c *OAuthClient) migrateOldTokens() {
	entries, err := os.ReadDir(tokensDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check for old .json format (not .enc)
		if strings.HasSuffix(name, ".json") {
			oldPath := filepath.Join(tokensDir, name)
			data, err := os.ReadFile(oldPath)
			if err != nil {
				continue
			}

			// Try to parse as old AccountToken format
			var token AccountToken
			if err := json.Unmarshal(data, &token); err != nil {
				continue
			}

			// Skip if email is empty (invalid token)
			if token.Email == "" {
				continue
			}

			// Save in new encrypted format
			if err := c.saveAccountToken(&token); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to migrate token for %s: %v\n", token.Email, err)
				continue
			}

			// Remove old file
			os.Remove(oldPath)
			fmt.Printf("Migrated token for %s to encrypted format\n", token.Email)
		}
	}
}

// ListAccounts returns all stored account emails
func (c *OAuthClient) ListAccounts() ([]string, error) {
	if err := os.MkdirAll(tokensDir, 0700); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(tokensDir)
	if err != nil {
		return nil, err
	}

	var accounts []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".enc") {
			// Read and decrypt to get email
			data, err := os.ReadFile(filepath.Join(tokensDir, entry.Name()))
			if err != nil {
				continue
			}
			decrypted, err := crypto.Decrypt(data)
			if err != nil {
				continue
			}
			var token AccountToken
			if err := json.Unmarshal(decrypted, &token); err != nil {
				continue
			}
			accounts = append(accounts, token.Email)
		}
	}

	return accounts, nil
}

// RemoveAccount removes a stored account
func (c *OAuthClient) RemoveAccount(email string) error {
	tokenPath := c.tokenPath(email)
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		return fmt.Errorf("account %s not found", email)
	}
	return os.Remove(tokenPath)
}

// GetAccessToken returns a valid access token for a specific account
func (c *OAuthClient) GetAccessToken(ctx context.Context, email string, forceReauth bool) (string, *UserInfo, error) {
	// Try to load cached token for this account
	if !forceReauth && email != "" {
		accountToken, err := c.loadAccountToken(email)
		if err == nil {
			token := c.toOAuth2Token(accountToken)
			// Check if token needs refresh
			if token.Expiry.Before(time.Now()) {
				newToken, err := c.refreshToken(ctx, token)
				if err == nil {
					accountToken.AccessToken = newToken.AccessToken
					accountToken.Expiry = newToken.Expiry
					c.saveAccountToken(accountToken)
					return newToken.AccessToken, &UserInfo{Email: accountToken.Email}, nil
				}
				// Refresh failed, need to reauth
			} else {
				return token.AccessToken, &UserInfo{Email: accountToken.Email}, nil
			}
		}
	}

	// Need to authenticate
	token, err := c.authenticate(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Get user info to identify the account
	userInfo, err := c.GetUserInfo(ctx, token.AccessToken)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Save token with email
	accountToken := &AccountToken{
		Email:        userInfo.Email,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}

	if err := c.saveAccountToken(accountToken); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save token: %v\n", err)
	}

	return token.AccessToken, userInfo, nil
}

// GetAllAccountTokens returns access tokens for all stored accounts
func (c *OAuthClient) GetAllAccountTokens(ctx context.Context) ([]struct {
	Email       string
	AccessToken string
}, error) {
	accounts, err := c.ListAccounts()
	if err != nil {
		return nil, err
	}

	var results []struct {
		Email       string
		AccessToken string
	}

	for _, email := range accounts {
		accessToken, _, err := c.GetAccessToken(ctx, email, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get token for %s: %v\n", email, err)
			continue
		}
		results = append(results, struct {
			Email       string
			AccessToken string
		}{Email: email, AccessToken: accessToken})
	}

	return results, nil
}

// authenticate performs the OAuth flow
func (c *OAuthClient) authenticate(ctx context.Context) (*oauth2.Token, error) {
	// Generate state for CSRF protection
	state := fmt.Sprintf("%d", time.Now().UnixNano())

	// Channel to receive the authorization code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Create a new ServeMux to avoid handler conflicts
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", callbackPort),
		Handler: mux,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errChan <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			http.Error(w, "No code received", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #1a1a2e;">
<div style="text-align: center; color: #fff;">
<h1 style="color: #4ade80;">✓ Authentication Successful</h1>
<p>You can close this window and return to the terminal.</p>
</div>
</body>
</html>`)
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Build authorization URL
	authURL := c.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If browser doesn't open, visit:\n%s\n\n", authURL)

	// Open browser
	openBrowser(authURL)

	// Wait for callback
	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		server.Shutdown(ctx)
		return nil, err
	case <-time.After(5 * time.Minute):
		server.Shutdown(ctx)
		return nil, fmt.Errorf("authentication timed out")
	}

	// Shutdown server
	server.Shutdown(ctx)

	// Exchange code for token
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	return token, nil
}

// refreshToken refreshes an expired token
func (c *OAuthClient) refreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	tokenSource := c.config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	return newToken, nil
}

// tokenPath returns the path to the token file for a given email
func (c *OAuthClient) tokenPath(email string) string {
	// Use hashed email for filename to avoid PII exposure
	hashedEmail := crypto.HashEmail(email)
	return filepath.Join(tokensDir, hashedEmail+".enc")
}

// loadAccountToken loads a token for a specific email
func (c *OAuthClient) loadAccountToken(email string) (*AccountToken, error) {
	data, err := os.ReadFile(c.tokenPath(email))
	if err != nil {
		return nil, err
	}

	// Decrypt the data
	decrypted, err := crypto.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	var token AccountToken
	if err := json.Unmarshal(decrypted, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// saveAccountToken saves a token for a specific email
func (c *OAuthClient) saveAccountToken(token *AccountToken) error {
	if err := os.MkdirAll(tokensDir, 0700); err != nil {
		return err
	}

	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	// Encrypt the data
	encrypted, err := crypto.Encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	return os.WriteFile(c.tokenPath(token.Email), encrypted, 0600)
}

// toOAuth2Token converts AccountToken to oauth2.Token
func (c *OAuthClient) toOAuth2Token(at *AccountToken) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  at.AccessToken,
		RefreshToken: at.RefreshToken,
		TokenType:    at.TokenType,
		Expiry:       at.Expiry,
	}
}

// openBrowser opens the URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}

	cmd.Start()
}

// GetUserInfo fetches the authenticated user's email and profile info
func (c *OAuthClient) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %s", string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}
