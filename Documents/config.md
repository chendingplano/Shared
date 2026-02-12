# Configuration Guide: Development and Production Environments

This document describes how to configure the development and production environments for applications using the shared library with Ory Kratos authentication.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Development Environment (USE_EMBED_FRONTEND=false)](#development-environment)
- [Production Environment (USE_EMBED_FRONTEND=true)](#production-environment)
- [Building and Restarting Services](#building-and-restarting-services)
- [Troubleshooting](#troubleshooting)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                         Browser                         │
└──────────────────────────┬──────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│  Kratos UI      │ │  Go Backend     │ │  Svelte Frontend│
│  (Port 4455)    │ │  (Port 8080)    │ │  (Port 8080/    │
│                 │ │                 │ │   embedded)     │
│  - Login        │ │  - API routes   │ │                 │
│  - Register     │ │  - Auth proxy   │ │  - SPA pages    │
│  - Settings     │ │  - Session mgmt │ │  - OAuth CB     │
└────────┬────────┘ └────────┬────────┘ └─────────────────┘
         │                   │
         ▼                   ▼
┌─────────────────────────────────────────┐
│           Ory Kratos (Port 4433/4434)   │
│                                         │
│  - Identity Management                  │
│  - Session Management                   │
│  - OAuth/OIDC (Google)                  │
└────────────────────┬────────────────────┘
                     │
                     ▼
            ┌─────────────────┐
            │   PostgreSQL    │
            │   (Port 5432)   │
            └─────────────────┘
```

**Key Environment Variable: `USE_EMBED_FRONTEND`**

| Value | Mode | Frontend Serving |
|-------|------|------------------|
| `""` (empty/unset) | Development | Go backend proxies to Vite dev server (port 8080) |
| `"true"` | Production | Go backend serves embedded static files |

---

## Development Environment

Development mode (`USE_EMBED_FRONTEND=false` or unset) runs the Svelte frontend on a separate Vite dev server with hot module replacement (HMR).

### 1. shared/ Library Configuration

The shared library reads configuration from environment variables. No `.env` file is needed in `shared/go/` itself; the consuming application provides the environment.

**Key environment variables used by `shared/go`:**

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_USE_KRATOS` | `"false"` | Set to `"true"` to enable Kratos authentication |
| `KRATOS_PUBLIC_URL` | `http://localhost:4433` | Kratos public API URL |
| `KRATOS_ADMIN_URL` | `http://localhost:4434` | Kratos admin API URL |
| `VITE_DEV_ONLY_URL` | - | Vite URL (e.g., `http://localhost:5173`), applicabled in dev environment only, used by Go |
| `VITE_DEFAULT_NORM_ROUTE` | `/dashboard` | Default redirect after login |
| `VITE_DEFAULT_ADMIN_ROUTE` | `/admin/dashboard` | Default redirect for admin users |
| `APP_BASE_URL` | `http://localhost:8080` | Default base URL |


### 2. Application Configuration (ChenWeb Example)

**File: `ChenWeb/.env`** (for development with separate frontend):

```bash
# Application URLs
VITE_DEV_ONLY_URL="http://localhost:5173"
VITE_DEFAULT_NORM_ROUTE="/dashboard"
VITE_DEFAULT_ADMIN_ROUTE="/admin/dashboard"

# Environment
APP_ENV="development"
SERVER_PORT="8080"

# Frontend mode: false/unset = proxy to Vite dev server
USE_EMBED_FRONTEND=""

# API Base URL (for Vite config)
VITE_API_BASE_URL=http://localhost:8080
API_BASE_URL=http://localhost:8080

# Auth configuration
VITE_USE_AUTH_STORE="true"
AUTH_USE_KRATOS="true"
KRATOS_PUBLIC_URL="http://127.0.0.1:4433"
KRATOS_ADMIN_URL="http://127.0.0.1:4434"

# PostgreSQL
USE_POSTGRESQL=true
PG_USER_NAME="admin"
PG_PASSWORD="plano4628"
PG_DB_NAME="miner"

# Google OAuth (for direct Google login, not via Kratos)
GOOGLE_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-your-secret
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/auth/google/callback
```

**File: `ChenWeb/mise.local.toml`** (local overrides):

```toml
[env]
AUTH_USE_KRATOS="true"                    # If "true", use Kratos for authentication
DATA_HOME_DIR="$HOME/Apps/ChenWebData"    # The data dir
DATA_SYNC_CONFIG="$HOME/.config/syncdata/config.toml"   # Used by syncdata
FILE_LOGGER="lumberjack"                  # The logger: ["lumberjack", "logfile"]
KRATOS_PUBLIC_URL="http://127.0.0.1:4433" # Kratos public URL
KRATOS_ADMIN_URL="http://127.0.0.1:4434"  # Kratos admin URL
LOG_FILE_DIR="$HOME/Apps/ChenWebLog"      # Log file dir
MAX_AGE_IN_DAYS="30"
MAX_SIZE_IN_MB="100"
NUM_LOG_FILES="10"
PG_PASSWORD="plano4628"
SHARED_LIB_CONFIG_DIR="$HOME/Workspace/shared/libconfig.toml"
```

### 3. Ory Kratos Configuration

**File: `Ory/kratos/kratos.yml`** (key sections for development):

```yaml
version: v1.3.1

dsn: postgres://admin:plano4628@127.0.0.1:5432/kratos?sslmode=disable

serve:
  public:
    base_url: http://localhost:4433/
    cors:
      enabled: true
      allowed_origins:
        - http://127.0.0.1:5173
        - http://localhost:5173
        - http://127.0.0.1:8080
        - http://localhost:8080
      allowed_methods:
        - POST
        - GET
        - PUT
        - PATCH
        - DELETE
      allowed_headers:
        - Authorization
        - Cookie
        - Content-Type
        - X-Session-Token
      exposed_headers:
        - Content-Type
        - Set-Cookie
      allow_credentials: true
  admin:
    base_url: http://0.0.0.0:4434/

selfservice:
  # Redirect to frontend's OAuth callback page
  default_browser_return_url: http://localhost:5173/oauth/callback
  allowed_return_urls:
    - http://localhost:5173
    - http://localhost:5173/oauth/callback
    - http://127.0.0.1:5173
    - http://127.0.0.1:5173/oauth/callback
    - http://localhost:8080
    - http://localhost:8080/oauth/callback

  methods:
    oidc:
      enabled: true
      config:
        providers:
          - id: google
            provider: google
            client_id: "your-google-client-id.apps.googleusercontent.com"
            client_secret: "GOCSPX-your-secret"
            mapper_url: "base64://..."  # Jsonnet mapper for claims
            scope:
              - email
              - profile

  flows:
    login:
      ui_url: http://localhost:4455/login
    registration:
      ui_url: http://localhost:4455/registration
    # ... other flows
```

Use the following command to generate base64 encoding for your mapper_url:
```bash
echo -n 'local claims = std.extVar('claims'); { identity: { traits: { [if "email" in claims && claims.email_verified then "email" else null]: claims.email, name: { first: if "given_name" in claims then claims.given_name else "", last: if "family_name" in claims then claims.family_name else "" } } } }' | base64
```

### 4. Development Workflow

```
Terminal 1: Kratos Identity Server
├── cd Ory
└── mise start-kratos

Terminal 2: Kratos Self-Service UI (optional)
├── cd Ory
└── mise start-kratos-ui

Terminal 3: Go Backend + Svelte Frontend
├── cd ChenWeb
└── mise dev
    ├── Runs Go backend on :8080
    └── Runs Vite dev server on :5173
```

**Access Points in Development:**

| Service | URL | Description |
|---------|-----|-------------|
| Frontend | http://localhost:8080| Svelte app with HMR |
| Go Backend | http://localhost:8080 | API endpoints |
| Kratos UI | http://localhost:4455 | Login/Register forms |
| Kratos Public API | http://localhost:4433 | Session validation |
| Kratos Admin API | http://localhost:4434 | Identity management |

---

## Production Environment

Production mode (`USE_EMBED_FRONTEND=true`) embeds the built frontend into the Go binary.

### 1. Application Configuration (ChenWeb Example)

**File: `ChenWeb/.env`** (for production):

```bash
# Application URLs - single port serves everything
APP_BASE_URL="http://localhost:8080"
VITE_DEFAULT_NORM_ROUTE="/dashboard"
VITE_DEFAULT_ADMIN_ROUTE="/admin/dashboard"

# Environment
APP_ENV="production"
SERVER_PORT="8080"

# Frontend mode: true = serve embedded static files
USE_EMBED_FRONTEND="true"

# API Base URL (same as app domain)
VITE_API_BASE_URL=http://localhost:8080
API_BASE_URL=http://localhost:8080

# Auth configuration
VITE_USE_AUTH_STORE="true"
AUTH_USE_KRATOS="true"
KRATOS_PUBLIC_URL="http://127.0.0.1:4433"
KRATOS_ADMIN_URL="http://127.0.0.1:4434"

# PostgreSQL
USE_POSTGRESQL=true
PG_USER_NAME="admin"
PG_PASSWORD="your-secure-password"
PG_DB_NAME="miner"

# Google OAuth - update redirect URL to match production port
GOOGLE_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-your-secret
GOOGLE_OAUTH_REDIRECT_URL=http://localhost:8080/auth/google/callback
```

### 2. Kratos Configuration Updates

Update `Ory/kratos/kratos.yml` for production port:

```yaml
selfservice:
  # Update to production port
  default_browser_return_url: http://localhost:8080/oauth/callback
  allowed_return_urls:
    - http://localhost:8080
    - http://localhost:8080/oauth/callback
    # Keep dev URLs if needed for local testing
    - http://localhost:5173
    - http://localhost:5173/oauth/callback

serve:
  public:
    cors:
      allowed_origins:
        - http://127.0.0.1:8080
        - http://localhost:8080
        # Keep dev origins if needed
        - http://127.0.0.1:5173
        - http://localhost:5173
```

### 3. Production Workflow

```bash
# Build frontend and embed into Go binary
cd ChenWeb
mise build-web      # Builds Svelte → web/build/ → server/api/webbuild/
mise build-server   # Builds Go binary with embedded frontend

# Start production server
mise serve
# Or directly:
USE_EMBED_FRONTEND=true ./.cache/server.exe serve --dir
```

---

## Building and Restarting Services

### Kratos (Identity Server)

```bash
cd /Users/cding/Workspace/Ory

# First-time setup (builds Kratos, creates DB, runs migrations, installs UI)
mise setup

# Build Kratos from source (after source code changes)
mise build-kratos

# Run database migrations
mise migrate

# Start Kratos server
mise start-kratos

# Start Kratos self-service UI (in separate terminal)
mise start-kratos-ui
```

**Restart Kratos after config changes:**

```bash
# Stop current process (Ctrl+C)
# Then restart:
mise start-kratos
```

### shared/ Library

The shared library is compiled into consuming applications. No separate build step is required.

**After modifying shared/go:**

```bash
cd /Users/cding/Workspace

# Sync Go workspace
go work sync

# Test changes
cd shared/go && go test ./...

# Rebuild consuming applications
cd ../ChenWeb && mise build-server
# or
cd ../tax && mise build-server
```

### Application (ChenWeb)

**Development mode:**

```bash
cd /Users/cding/Workspace/ChenWeb

# Install frontend dependencies
mise update   # runs: cd web && bun install

# Start development servers (Go + Vite)
mise dev
```

**Production mode:**

```bash
cd /Users/cding/Workspace/ChenWeb

# Step 1: Build frontend
mise build-web
# This runs:
#   cd web && bun run build
#   rsync web/build/ → server/api/webbuild/

# Step 2: Build Go binary with embedded frontend
mise build-server
# This runs:
#   go build -o ./.cache/server.exe ./server/cmd/deepdoc/.

# Step 3: Start production server
mise serve
# Or directly:
USE_EMBED_FRONTEND=true ./.cache/server.exe serve --dir

# Combined build (both frontend and server)
mise build-both
```

### Complete Restart Sequence

```bash
# 1. Stop all running services (Ctrl+C in each terminal)

# 2. Restart Kratos
cd /Users/cding/Workspace/Ory
mise start-kratos

# 3. Restart Kratos UI (optional, for self-service flows)
# In a new terminal:
cd /Users/cding/Workspace/Ory
mise start-kratos-ui

# 4. Restart Application
cd /Users/cding/Workspace/ChenWeb

# For development:
mise dev

# For production:
mise build-both && mise serve
```

---

## Troubleshooting

### Common Issues

**1. CORS Errors**

Ensure `allowed_origins` in `kratos.yml` includes all frontend URLs:
- Development: `http://localhost:8080`, `http://127.0.0.1:8080`
- Production: `http://localhost:8080`, `http://127.0.0.1:8080`

**2. Session Not Persisting**

- Check cookie domain matches between Kratos and application
- Ensure `allow_credentials: true` in CORS config
- Verify `SameSite` cookie setting is `Lax` (not `Strict`)

**3. OAuth Redirect Errors**

- Verify `default_browser_return_url` in `kratos.yml` matches `APP_BASE_URL`
- Check `allowed_return_urls` includes OAuth callback path
- Ensure Google OAuth redirect URL matches the current port

**4. Frontend Not Loading (Production)**

- Verify `USE_EMBED_FRONTEND="true"` is set
- Run `mise build-web` before `mise build-server`
- Check `server/api/webbuild/` contains built files

**5. 401 Unauthorized Errors**

- Check `KRATOS_PUBLIC_URL` points to running Kratos instance
- Verify `AUTH_USE_KRATOS="true"` is set
- Ensure session cookie is being sent with requests

### Environment Variable Reference

| Variable | Dev Value | Prod Value | Description |
|----------|-----------|------------|-------------|
| `USE_EMBED_FRONTEND` | `""` | `"true"` | Frontend serving mode |
| `VITE_DEV_ONLY_URL` | `http://localhost:5173` | Not Applicable | Vite URL, applicable in dev environment only, used by Go|
| `APP_BASE_URL` | `http://localhost:8080` | `https://<domainname>` | Application base URL|
| `SERVER_PORT` | `8080` | `8080` | Go backend port |
| `AUTH_USE_KRATOS` | `"true"` | `"true"` | Enable Kratos auth |
| `KRATOS_PUBLIC_URL` | `http://127.0.0.1:4433` | `http://127.0.0.1:4433` | Kratos public API |
| `KRATOS_ADMIN_URL` | `http://127.0.0.1:4434` | `http://127.0.0.1:4434` | Kratos admin API |
| `APP_ENV` | `development` | `production` | Environment mode |

---

## Quick Reference: Switching Environments

**Switch to Development:**
```bash
# In ChenWeb/.env:
VITE_DEV_ONLY_URL="http://localhost:5173"
USE_EMBED_FRONTEND=""

# In Ory/kratos/kratos.yml:
default_browser_return_url: http://localhost:8080/oauth/callback

# Start services:
cd Ory && mise start-kratos
cd ChenWeb && mise dev
```

**Switch to Production:**
```bash
# In ChenWeb/.env:
APP_BASE_URL="http://localhost:8080"
USE_EMBED_FRONTEND="true"

# In Ory/kratos/kratos.yml:
default_browser_return_url: http://localhost:8080/oauth/callback

# Build and start:
cd Ory && mise start-kratos
cd ChenWeb && mise build-both && mise serve
```
