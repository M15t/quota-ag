# AGENTS.md

This file provides guidance for AI coding agents working in this repository.

## Project Overview

`quota-ag` is a CLI tool for monitoring Google Antigravity API quota usage. It uses OAuth2 authentication and provides colorized terminal output with quota status for AI models.

## Build/Lint/Test Commands

### Build

```bash
# Build the binary
make build
# or
go build -o quota-ag .
```

### Run

```bash
# Run with watch mode (default development flow)
make start

# Run directly
./quota-ag --all
```

### Lint

```bash
# Run Go linter (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
golangci-lint run

# Format code
go fmt ./...

# Vet code
go vet ./...
```

### Test

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a single test by name
go test -v -run TestFunctionName ./...

# Run tests in a specific package
go test -v ./internal/crypto/...

# Run tests with coverage
go test -cover ./...
```

### Clean

```bash
make clean
```

## Code Style Guidelines

### Package Structure

```
quota-ag/
├── main.go                 # CLI entry point, flag parsing, display logic
├── internal/
│   ├── auth/               # OAuth2 authentication, token management
│   ├── client/             # API client for Google Cloud Code
│   ├── crypto/             # Encryption utilities (AES-256-GCM)
│   └── models/             # Shared data structures
```

### Import Organization

Imports must be grouped in this order, separated by blank lines:

1. Standard library
2. Internal packages (`quota-ag/internal/...`)
3. External dependencies

```go
import (
    "context"
    "encoding/json"
    "fmt"

    "quota-ag/internal/auth"
    "quota-ag/internal/models"

    "golang.org/x/oauth2"
)
```

### Formatting

- Use `go fmt` for all formatting
- Use tabs for indentation (Go standard)
- Maximum line length: 100 characters (soft limit)
- No trailing whitespace

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Exported types | PascalCase | `CloudCodeClient`, `QuotaStatus` |
| Unexported types | camelCase | `accountToken` |
| Exported functions | PascalCase | `NewCloudCodeClient`, `FetchQuota` |
| Unexported functions | camelCase | `loadProjectInfo`, `doRequest` |
| Constants (exported) | PascalCase | `DefaultTimeout` |
| Constants (unexported) | camelCase | `baseURL`, `maxRetries` |
| Variables | camelCase | `httpClient`, `accessToken` |

### Type Definitions

- Use struct tags for JSON serialization with `json:"field_name"`
- Use `omitempty` for optional fields
- Group related types in the same file

```go
type ModelQuota struct {
    Name         string    `json:"name"`
    ModelID      string    `json:"model_id"`
    RemainingPct float64   `json:"remaining_pct"`
    ResetTime    time.Time `json:"reset_time"`
    ResetIn      string    `json:"reset_in,omitempty"`
}
```

### Error Handling

- Always wrap errors with context using `fmt.Errorf`
- Use `%w` verb for error wrapping to preserve error chain
- Check errors immediately after function calls
- Return early on errors

```go
if err != nil {
    return nil, fmt.Errorf("failed to load project info: %w", err)
}
```

For user-facing errors, write to stderr:

```go
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
os.Exit(1)
```

### Comments

- All exported types and functions must have doc comments
- Comments should be complete sentences starting with the function/type name
- Use `//` for single-line comments, not `/* */`

```go
// CloudCodeClient handles API calls to Google Cloud Code
type CloudCodeClient struct { ... }

// NewCloudCodeClient creates a new Cloud Code API client
func NewCloudCodeClient(accessToken string) *CloudCodeClient { ... }
```

### Context Usage

- Pass `context.Context` as the first parameter to functions that do I/O
- Use `context.Background()` in main entry points
- Use `http.NewRequestWithContext` for HTTP requests

```go
func (c *CloudCodeClient) FetchQuota(ctx context.Context) (*models.QuotaStatus, error)
```

### HTTP Client Patterns

- Always set timeouts on HTTP clients
- Use retries with exponential backoff for transient errors
- Set appropriate headers (Authorization, User-Agent, Content-Type)

```go
httpClient: &http.Client{
    Timeout: 30 * time.Second,
}
```

### Security Considerations

- Never log or print access tokens or secrets
- Use encrypted storage for sensitive data (see `internal/crypto`)
- Hash email addresses for filenames to avoid PII exposure
- Set restrictive file permissions (0600 for tokens, 0700 for directories)

### Flag Parsing

- Use the standard `flag` package
- Define flags at the top of main()
- Use descriptive help text

```go
jsonOutput := flag.Bool("json", false, "Output in JSON format")
watch := flag.Duration("watch", 0, "Auto-refresh interval (e.g., 30s, 1m)")
```

## Dependencies

- `golang.org/x/oauth2` - OAuth2 authentication
- Standard library only for crypto, HTTP, and JSON operations

## Common Tasks

### Adding a New API Endpoint

1. Add request/response types to `internal/models/types.go`
2. Add the API method to `internal/client/cloudcode.go`
3. Use `doRequest` helper for consistent error handling and retries

### Adding a New CLI Flag

1. Define the flag in `main.go` with `flag.Bool/String/Duration`
2. Handle the flag logic after `flag.Parse()`
3. Update the Makefile if a common workflow is needed
