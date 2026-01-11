# quota-ag

A command-line tool for monitoring Google Antigravity API quota usage. Authenticate with your Google accounts and check the remaining quota for various AI models available through Google Cloud Code / Antigravity services.

## Features

- **OAuth2 Authentication** - Browser-based Google login flow
- **Multi-Account Support** - Add, list, and remove multiple Google accounts
- **Quota Monitoring** - View quota status for individual or all accounts
- **Watch Mode** - Auto-refresh with configurable intervals
- **Visual Output** - Colorized terminal output with progress bars
- **JSON Export** - Structured output for scripting and automation
- **Secure Storage** - Encrypted local token storage using AES-256-GCM

## Installation

### Prerequisites

- Go 1.24 or later

### Build from Source

```bash
# Clone the repository
git clone https://github.com/anomalyco/quota-ag.git
cd quota-ag

# Build the binary
make build
# or
go build -o quota-ag .
```

## Usage

### Adding an Account

Opens a browser for Google OAuth authentication:

```bash
./quota-ag --add
```

### Viewing Quota

```bash
# Show quota for default account
./quota-ag

# Show quota for all accounts
./quota-ag --all

# Show quota for a specific account
./quota-ag --account user@gmail.com
```

### Watch Mode

Continuously monitor quota with auto-refresh:

```bash
# Refresh every minute
./quota-ag --all --watch 1m

# Refresh every 30 seconds
./quota-ag --watch 30s
```

### Account Management

```bash
# List all stored accounts
./quota-ag --list

# Remove an account (interactive)
./quota-ag --remove

# Remove a specific account
./quota-ag --remove user@gmail.com
```

### JSON Output

For scripting and automation:

```bash
./quota-ag --json
./quota-ag --all --json
```

## Command Reference

| Flag | Description |
|------|-------------|
| `--add` | Add a new Google account |
| `--list` | List all stored accounts |
| `--remove [email]` | Remove an account (interactive if no email provided) |
| `--account <email>` | Use a specific account |
| `--all` | Show quota for all accounts |
| `--watch <duration>` | Enable watch mode with refresh interval (e.g., `1m`, `30s`) |
| `--json` | Output in JSON format |

## Makefile Targets

| Command | Description |
|---------|-------------|
| `make build` | Build the binary |
| `make start` | Build and run with `--all --watch 1m` |
| `make add` | Add a new account |
| `make list` | List all accounts |
| `make remove` | Remove an account |
| `make all` | Show quota for all accounts |
| `make json` | Output in JSON format |
| `make clean` | Remove build artifacts |

## Security

- OAuth2 tokens are encrypted at rest using **AES-256-GCM**
- Encryption key is derived from machine-specific identifiers
- Email addresses are hashed (SHA256) for filename privacy
- Tokens are stored in `.tokens/` directory (git-ignored)

## Project Structure

```
quota-ag/
├── internal/
│   ├── auth/        # OAuth2 authentication and token management
│   ├── client/      # Google Cloud Code API client
│   ├── crypto/      # AES-256-GCM encryption utilities
│   └── models/      # Data structures for API requests/responses
├── main.go          # CLI entry point and display logic
├── go.mod           # Go module definition
├── Makefile         # Build automation
└── LICENSE          # MIT License
```

## License

MIT License - see [LICENSE](LICENSE) for details.
