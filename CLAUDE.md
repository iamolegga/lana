# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Usage Instructions

Always use context7 when I need code generation, setup or configuration steps, or
library/API documentation. This means you should automatically use the Context7 MCP
tools to resolve library id and get library docs without me having to explicitly ask.


## Project Overview

Lana is a production-ready OAuth SSO authentication server written in Go. It provides OAuth 2.0 authentication through multiple providers (Google, Facebook) and issues JWTs for authenticated users. The server supports multi-host configurations with host-specific JWT signing keys and provider settings.

## Development Commands

### Build & Run
- `task air` - Run development server with hot reload (uses Air)
- `go build -o ./tmp/main ./cmd/server` - Build the server binary
- `./tmp/main -config config.yaml` - Run the server with a specific config file

### Code Quality
- `task fmt` - Format code with `go fmt`
- `task vet` - Run static analysis with `go vet`
- `task test` - Run all tests with verbose output
- `task tidy` - Clean up dependencies with `go mod tidy`

### Security Setup
- `task genKey` - Generate RSA private key for JWT signing (outputs to `./dev/private.pem`)
- `task genCookieSecret` - Generate a 32-character cookie secret for AES-256 encryption

### Configuration
- Copy `config.example.yaml` to `config.yaml` and configure:
  - Environment (development/production)
  - Server port, cookie settings, rate limiting
  - Logging level (debug/info/warn/error) and format (json/text)
  - Host-specific settings (login directory, JWT config, OAuth providers)
  - Environment variables are expanded using `$VAR_NAME` syntax

## Architecture

### Core Components

**Entry Point** ([cmd/server/main.go](cmd/server/main.go))
- Loads configuration from YAML file (with environment variable substitution)
- Initializes structured logging (slog)
- Loads RSA private keys for JWT signing (one per host)
- Creates rate limiter with configurable requests per minute
- Registers OAuth provider factories in a registry pattern
- Instantiates providers from configuration
- Starts HTTP server with graceful shutdown handling

**Configuration** ([internal/config/config.go](internal/config/config.go))
- YAML-based configuration with validation (using go-playground/validator)
- Supports environment variable substitution via `$VAR_NAME` syntax
- Multi-host support: each host can have its own JWT keys, providers, and login directory
- Validation ensures required fields and proper formats (URLs, file paths, durations)

**HTTP Server** ([internal/server/server.go](internal/server/server.go))
- Standard library HTTP server with timeouts (read: 15s, write: 15s, idle: 60s)
- Routes:
  - `GET /.well-known/jwks.json` - JSON Web Key Set for JWT verification
  - `GET /oauth/login/{provider}` - Initiate OAuth flow
  - `GET /oauth/callback/{provider}` - OAuth callback handler
  - `GET /` - Root handler (serves login page)
- All routes wrapped with rate limiting middleware
- Host-aware: uses `Host` header to determine which config/keys to use

**OAuth System** ([internal/oauth/provider.go](internal/oauth/provider.go))
- Provider interface defines: `GetAuthURL()`, `ExchangeCode()`, `GetUser()`, `Name()`
- Registry pattern for pluggable providers
- Factory functions register providers by name
- Current implementations:
  - [internal/providers/google/google.go](internal/providers/google/google.go) - Google OAuth
  - [internal/providers/facebook/facebook.go](internal/providers/facebook/facebook.go) - Facebook OAuth

**Rate Limiting** ([internal/ratelimit/limiter.go](internal/ratelimit/limiter.go))
- Per-IP rate limiting using token bucket algorithm
- Configurable requests per minute and cleanup interval
- Supports X-Forwarded-For header parsing with configurable index (for proxy setups)
- Uses context for graceful cleanup on shutdown

**Graceful Shutdown** ([internal/server/graceful_shutdown.go](internal/server/graceful_shutdown.go))
- Subscribes to OS signals (SIGINT, SIGTERM)
- Maintains server base context for cleanup coordination
- Provides controlled shutdown with timeout

**State Management** ([internal/server/state.go](internal/server/state.go))
- OAuth state generation and validation
- Encrypted state cookies using AES-256
- Prevents CSRF attacks

**Logging** ([internal/logging/setup.go](internal/logging/setup.go))
- Structured logging using `log/slog` with configurable level and format
- Environment-aware formatting:
  - Development: Uses console-slog for human-readable colored output
  - Production: JSON or text handlers for machine-parseable logs
- Source location tracking with path trimming for cleaner output
- UTC timestamps in ISO 8601 format (JSON/text) or HH:MM:SS (console)

### Key Design Patterns

1. **Registry Pattern**: OAuth providers are registered via factory functions, making it easy to add new providers
2. **Multi-Host Architecture**: Single server instance can handle multiple hosts with different configurations
3. **Structured Logging**: Uses `log/slog` with contextual fields throughout
4. **Graceful Shutdown**: Server context propagates to all components for coordinated cleanup
5. **Middleware Composition**: Rate limiting and future middleware applied via wrapper functions

### Adding a New OAuth Provider

1. Create new package under `internal/providers/{provider_name}/`
2. Implement the `oauth.Provider` interface
3. Create a factory function: `func New(cfg *config.OAuthProvider) (oauth.Provider, error)`
4. Register in [cmd/server/main.go](cmd/server/main.go): `registry.Register("provider_name", provider.New)`
5. Add provider config to host in `config.yaml`

### JWT Flow

1. User initiates login via `/oauth/login/{provider}`
2. Server generates encrypted state, stores in cookie, redirects to OAuth provider
3. Provider redirects back to `/oauth/callback/{provider}` with code
4. Server validates state, exchanges code for tokens, fetches user info
5. Server generates JWT signed with host-specific RSA private key
6. JWT includes standard claims (sub, aud, exp, iat) plus custom user data
7. Public keys exposed at `/.well-known/jwks.json` for verification by downstream services

### Important Implementation Details

- JWT signing uses RSA with PKCS#1 format (PEM block type: "RSA PRIVATE KEY")
- Cookie encryption uses the secret from config (must be 32+ chars for AES-256)
- Rate limiting tracks IPs from X-Forwarded-For header with configurable index
- Configuration validation happens at startup, server won't start with invalid config
- All handlers receive host-specific config based on the HTTP Host header
