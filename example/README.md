# LANA Example Deployment

This directory contains a complete production-like deployment example for LANA OAuth SSO server. It demonstrates how to deploy LANA with Docker Compose, Traefik reverse proxy, HTTPS support, and an example client application.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        Browser (HTTPS)                           │
└──────────────────────┬───────────────────────────────────────────┘
                       │
                       │ https://auth.lanaexample.dev
                       │ https://app.lanaexample.dev
                       │
                       ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Traefik Reverse Proxy                         │
│              (SSL/TLS Termination, Routing)                      │
└──────────────┬────────────────────────────┬──────────────────────┘
               │                            │
               │ http://lana:8080           │ http://app:8080
               │                            │
               ▼                            ▼
┌──────────────────────────┐    ┌──────────────────────────────────┐
│    LANA OAuth Server     │    │   Example Client Application     │
│   (Container: lana)      │    │      (Container: app)            │
│                          │    │                                  │
│ - OAuth 2.0 flows        │◄───┤ - JWT verification (JWKS)        │
│ - JWT token issuance     │    │ - Session management             │
│ - JWKS endpoint          │    │ - User profile display           │
│ - Prometheus metrics     │    │                                  │
└──────────────────────────┘    └──────────────────────────────────┘
```

## What's Included

### 1. **Docker Compose Setup** ([docker-compose.yml](docker-compose.yml))

Three-service architecture:
- **Traefik** - Reverse proxy with HTTPS termination and routing
- **LANA** - OAuth authentication server
- **App** - Example client application demonstrating LANA integration

### 2. **Traefik Configuration** ([ingress/traefik/](ingress/traefik/))

- SSL/TLS certificate management
- Dynamic routing configuration
- HTTP to HTTPS redirection
- Host-based routing (auth.lanaexample.dev, app.lanaexample.dev)

### 3. **LANA Configuration** ([lana/config.yaml](lana/config.yaml))

- Production-ready configuration
- Google OAuth provider setup
- Prometheus metrics enabled
- Wildcard redirect URL support
- Rate limiting configured

### 4. **Example Client App** ([app/](app/))

A complete OAuth client implementation demonstrating:
- JWT verification using JWKS
- Secure session management
- User profile display
- See [app/README.md](app/README.md) for detailed documentation

### 5. **Development Tools** ([Taskfile.yml](Taskfile.yml))

Automated tasks for local development:
- Certificate generation with mkcert
- JWT RSA key generation
- /etc/hosts management with hostctl
- Docker Compose lifecycle management

## Prerequisites

- **Docker & Docker Compose** (v20.10+)
- **mkcert** - Local HTTPS certificates ([installation](https://github.com/FiloSottile/mkcert#installation))
- **hostctl** - /etc/hosts management ([installation](https://github.com/guumaster/hostctl#installation))
- **Task** - Task runner, optional ([installation](https://taskfile.dev/installation/))
- **Google OAuth Credentials** - From [Google Cloud Console](https://console.cloud.google.com/)
  - Redirect URI: `https://auth.lanaexample.dev/oauth/callback/google`

## Quick Start

### Step 1: Generate Certificates

Generate SSL certificates for HTTPS:

```bash
cd example

# Generate wildcard certificate for *.lanaexample.dev
task certs:web

# This creates:
# - ingress/certs/_wildcard.lanaexample.dev.pem
# - ingress/certs/_wildcard.lanaexample.dev-key.pem
# - ingress/certs/rootCA.pem
```

### Step 2: Generate JWT Keys

Generate RSA private key for JWT signing:

```bash
# Generate 2048-bit RSA key
task certs:jwt

# This creates:
# - lana/certs/private.pem
```

### Step 3: Configure /etc/hosts

Add local DNS entries:

```bash
# Add entries from .etchosts
task hosts:on

# This adds:
# 127.0.0.1 auth.lanaexample.dev
# 127.0.0.1 app.lanaexample.dev
# 127.0.0.1 traefik.lanaexample.dev
```

To remove later:
```bash
task hosts:off
```

### Step 4: Configure Environment Variables

Create a `.env` file with your OAuth credentials:

```bash
cp .env.example .env
```

Edit `.env` and add your OAuth provider credentials:

```bash
# Google OAuth (required)
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret

# Cookie secret is already set in docker-compose.yml
# You can override it here if needed:
# COOKIE_SECRET=your-32-character-secret-key-here
```

### Step 5: Start Services

```bash
# Build and start all services
task up
```

This will:
1. Build the LANA Docker image from the parent directory
2. Build the example app Docker image
3. Start Traefik, LANA, and the example app
4. Traefik will route requests based on hostname

### Step 6: Test the Setup

1. **Open the example app:**
   ```
   https://app.lanaexample.dev
   ```

2. **You'll be redirected to LANA for authentication:**
   ```
   https://auth.lanaexample.dev/oauth/login/google
   ```

3. **Complete Google OAuth authentication**

4. **You'll be redirected back to the app** with your profile displayed

### Step 7: Explore Endpoints

**LANA Endpoints:**
- JWKS: `https://auth.lanaexample.dev/.well-known/jwks.json`
- Metrics: `https://auth.lanaexample.dev/metrics`
- Login page: `https://auth.lanaexample.dev/`

**App Endpoints:**
- Home: `https://app.lanaexample.dev/`
- Logout: `https://app.lanaexample.dev/logout`

**Traefik Dashboard:**
- Dashboard: `https://traefik.lanaexample.dev/` (if enabled in traefik config)

## How the Example App Works

The example client application demonstrates a complete OAuth 2.0 flow:

1. User visits the app → checks for authentication cookie
2. If not authenticated → redirects to LANA OAuth server
3. User authenticates with OAuth provider (Google)
4. LANA redirects back with JWT token as query parameter
5. App fetches JWKS from LANA's `/.well-known/jwks.json`
6. App verifies JWT signature using public key from JWKS
7. App creates signed session cookie with user email
8. User sees authenticated welcome page

**Security Features:**
- **JWT Verification** - RSA signature verification using JWKS
- **Signed Cookies** - HMAC-SHA256 prevents cookie tampering
- **HTTP-only Cookies** - Prevents XSS attacks
- **SameSite Protection** - CSRF protection via SameSite=Lax

See [app/main.go](app/main.go) for the complete implementation.

## Stopping Services

```bash
# Stop all services
task down

# Or without Task:
docker compose down

# Stop and remove volumes
docker compose down -v
```

## Troubleshooting

### Certificate not trusted
Ensure mkcert's root CA is installed:
```bash
mkcert -install
```

### Cannot resolve domains
Check /etc/hosts entries:
```bash
hostctl list
task hosts:on
```

### LANA container fails to start
1. Check logs: `docker logs lana`
2. Verify OAuth credentials: `cat .env | grep GOOGLE`
3. Verify JWT key exists: `ls -la lana/certs/private.pem`

### Rate limiting during testing
Increase rate limit in `lana/config.yaml` and restart:
```yaml
ratelimit:
  requests_per_minute: 60  # Increase from 5
```

## Directory Structure

```
example/
├── README.md                    # This file
├── docker-compose.yml           # Multi-service setup
├── Taskfile.yml                 # Development automation
├── .env                         # Environment variables (gitignored)
├── .env.example                 # Environment template
├── .etchosts                    # Local DNS entries
│
├── lana/                        # LANA configuration
│   ├── config.yaml              # LANA server config
│   ├── certs/
│   │   └── private.pem          # JWT signing key (generated)
│   └── login/
│       └── index.html           # Login page template
│
├── ingress/                     # Traefik configuration
│   ├── traefik/
│   │   ├── traefik.yml          # Static configuration
│   │   └── dynamic.yml          # Dynamic routing
│   └── certs/
│       ├── _wildcard.lanaexample.dev.pem        # SSL cert (generated)
│       ├── _wildcard.lanaexample.dev-key.pem    # SSL key (generated)
│       └── rootCA.pem                           # Root CA (generated)
│
└── app/                         # Example client application
    ├── README.md                # App-specific documentation
    ├── Dockerfile               # App container image
    ├── main.go                  # App source code
    ├── go.mod                   # App dependencies
    └── go.sum                   # Dependency checksums
```
