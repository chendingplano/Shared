# Authentication Documentation

This document describes the authentication system for applications in this workspace, using **Ory Kratos** as the identity provider with support for email/password and OAuth-based authentication.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Supported Authentication Methods](#supported-authentication-methods)
  - [Sign Up with Email](#sign-up-with-email)
  - [Sign Up with Username and Password](#sign-up-with-username-and-password)
  - [Sign Up with Google](#sign-up-with-google)
  - [Login with Email](#login-with-email)
  - [Login with Username and Password](#login-with-username-and-password)
  - [Login with Google](#login-with-google)
  - [Forgot Password](#forgot-password)
- [Implementation Details](#implementation-details)
  - [Backend (Go)](#backend-go)
  - [Frontend (Svelte)](#frontend-svelte)
- [Configuration](#configuration)
- [Security Features](#security-features)
- [API Reference](#api-reference)
- [Related Documentation](#related-documentation)

---

## Overview

The authentication system is built on **Ory Kratos**, an open-source identity and user management system. Kratos handles:

- Identity management (users, credentials)
- Session management
- Password hashing and validation
- Email verification
- Password recovery
- OAuth/OIDC integration (Google, GitHub)
- Two-factor authentication (TOTP)

**Current Integration Status:** ChenWeb is in the process of integrating authentication with Ory Kratos.

---

## Architecture

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
│  - Password Hashing                     │
│  - Email Verification                   │
└────────────────────┬────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│   PostgreSQL    │     │   Resend SMTP   │
│   (Port 5432)   │     │                 │
│                 │     │   Email         │
│   Stores:       │     │   delivery      │
│   - Identities  │     │                 │
│   - Sessions    │     │                 │
└─────────────────┘     └─────────────────┘
```

---

## Supported Authentication Methods

### Sign Up with Email

Users can register a new account using their email address and password.

**Flow:**

1. User navigates to registration page
2. User enters email, password, first name, and last name
3. Frontend POSTs to `/auth/email/signup`
4. Backend creates a Kratos registration flow
5. Backend submits user data to Kratos
6. Kratos validates password strength
7. Kratos creates identity and sends verification email
8. User is redirected to login or dashboard (if auto-login enabled)

**Required Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `email` | string | Valid email address (primary identifier) |
| `password` | string | Must meet password strength requirements |
| `passwordConfirm` | string | Must match password |
| `first_name` | string | User's first name |
| `last_name` | string | User's last name |

**Backend Handler:** `HandleEmailSignupKratos` in `shared/go/api/auth/kratos.go`

**Example Request:**

```json
POST /auth/email/signup
{
  "email": "user@example.com",
  "password": "SecureP@ssw0rd!",
  "passwordConfirm": "SecureP@ssw0rd!",
  "first_name": "John",
  "last_name": "Doe"
}
```

---

### Sign Up with Username and Password

Similar to email signup, but uses username as an additional identifier.

**Note:** The current Kratos identity schema uses email as the primary identifier. Username support requires extending the identity schema in `Ory/kratos/identity.schema.json`.

**To Enable Username Support:**

1. Update identity schema to include username trait:

```json
{
  "username": {
    "type": "string",
    "title": "Username",
    "minLength": 3,
    "maxLength": 32,
    "pattern": "^[a-zA-Z0-9_]+$"
  }
}
```

2. Configure Kratos to allow login with username in `kratos.yml`:

```yaml
selfservice:
  methods:
    password:
      config:
        identifier_similarity_check_enabled: true
```

---

### Sign Up with Google

Users can register using their Google account via OAuth 2.0 / OIDC.

**Flow:**

1. User clicks "Sign up with Google" button
2. Frontend redirects to `/auth/google/login`
3. Backend initiates Kratos OIDC flow
4. Backend returns auto-submitting form to Kratos
5. Kratos redirects to Google OAuth consent screen
6. User authenticates with Google
7. Google redirects back to Kratos callback
8. Kratos creates/links identity with Google claims
9. Kratos redirects to `/oauth/callback` on frontend
10. Frontend receives session and redirects to dashboard

**Backend Handler:** `HandleGoogleLoginKratos` in `shared/go/api/auth/kratos.go`

**Kratos Configuration:**

```yaml
selfservice:
  methods:
    oidc:
      enabled: true
      config:
        providers:
          - id: google
            provider: google
            client_id: "your-google-client-id.apps.googleusercontent.com"
            client_secret: "GOCSPX-your-secret"
            mapper_url: "base64://..."
            scope:
              - email
              - profile
```

---

### Login with Email

Users can log in using their registered email and password.

**Flow:**

1. User navigates to login page
2. User enters email and password
3. Frontend POSTs to `/auth/email/login`
4. Backend creates Kratos login flow
5. Backend submits credentials to Kratos
6. Kratos validates credentials
7. Kratos returns session token
8. Backend checks for 2FA requirement
9. If 2FA required, returns `status: "2fa_required"` with redirect to `/verify-2fa`
10. Otherwise, returns session info and redirect URL
11. Frontend redirects to dashboard

**Backend Handler:** `HandleEmailLoginKratos` in `shared/go/api/auth/kratos.go`

**Example Request:**

```json
POST /auth/email/login
{
  "email": "user@example.com",
  "password": "SecureP@ssw0rd!"
}
```

**Example Response (Success):**

```json
{
  "status": "success",
  "session": {
    "id": "session-uuid",
    "identity": {
      "id": "identity-uuid",
      "traits": {
        "email": "user@example.com",
        "name": {
          "first": "John",
          "last": "Doe"
        }
      }
    }
  },
  "redirect_url": "/dashboard"
}
```

**Example Response (2FA Required):**

```json
{
  "status": "2fa_required",
  "redirect_url": "/verify-2fa"
}
```

---

### Login with Username and Password

Similar to email login, but uses username as the identifier.

**Note:** Requires username support to be enabled in the identity schema (see [Sign Up with Username and Password](#sign-up-with-username-and-password)).

**Example Request:**

```json
POST /auth/email/login
{
  "email": "johndoe",  // Username used as identifier
  "password": "SecureP@ssw0rd!"
}
```

---

### Login with Google

Users can log in using their linked Google account.

**Flow:**

Same as [Sign Up with Google](#sign-up-with-google). If the Google account is already linked to an existing identity, Kratos logs the user in. If not, Kratos creates a new identity.

**Frontend Trigger:**

```javascript
window.location.href = '/auth/google/login';
```

Make sure you registered your URLs with Google:
```url
https://console.cloud.google.com/welcome?project=gen-lang-client-0276721966
```
---

### Forgot Password

Users can reset their password via email.

**Flow:**

1. User navigates to login page and clicks "Forgot Password"
2. User enters their email address
3. Frontend POSTs to `/auth/email/forgot`
4. Backend initiates Kratos recovery flow
5. Kratos sends password reset email via Resend SMTP
6. User clicks link in email
7. User is redirected to password reset page
8. User enters new password
9. Kratos updates password and redirects to login

**Backend Handler:** `HandleForgotPasswordKratos` in `shared/go/api/auth/kratos.go`

**Example Request:**

```json
POST /auth/email/forgot
{
  "email": "user@example.com",
  "location": "ARX_LGN_090"
}
```

**Kratos Self-Service Flow:**

- **Recovery UI:** http://127.0.0.1:4455/recovery
- **Supported Methods:** Email link, recovery codes

---

## Implementation Details

### Backend (Go)

**Key Files:**

| File | Location | Purpose |
|------|----------|---------|
| Kratos Integration | `shared/go/api/auth/kratos.go` | Kratos API handlers (login, signup, logout, OAuth) |
| Auth Middleware | `shared/go/authmiddleware/auth.go` | Session validation middleware |
| Rate Limiting | `shared/go/api/auth/rate_limiter.go` | Login attempt rate limiting |
| Password Validation | `shared/go/api/auth/password_validation.go` | Password strength checks |
| CSRF Protection | `shared/go/api/auth/csrf.go` | Cross-site request forgery protection |
| OAuth Nonce | `shared/go/api/auth/oauth_nonce.go` | OAuth state/nonce management |
| Google OAuth | `shared/go/api/auth/google.go` | Google OAuth configuration |
| GitHub OAuth | `shared/go/api/auth/github.go` | GitHub OAuth configuration |

**Kratos Client Initialization:**

```go
import "github.com/chendingplano/shared/go/api/auth"

// Initialize Kratos client
kratosPublicURL := os.Getenv("KRATOS_PUBLIC_URL") // default: http://localhost:4433
auth.InitKratosClient(kratosPublicURL)
```

**Session Validation:**

```go
// Validate session with Kratos
session, err := auth.ValidateSession(echoContext)
if err != nil {
    return echo.NewHTTPError(http.StatusUnauthorized, "Invalid session")
}

// Access user identity
identity := session.Identity
traits := identity.Traits.(map[string]interface{})
email := traits["email"]
```

### Frontend (Svelte)

**Key Files:**

| File | Location | Purpose |
|------|----------|---------|
| Auth Store | `shared/svelte/src/lib/stores/auth.svelte.ts` | Svelte 5 auth state management |
| Login Component | `web/src/lib/components/login-01.svelte` | Login/signup/forgot UI |
| OAuth Callback | `web/src/routes/oauth/callback/+page.svelte` | OAuth redirect handler |
| 2FA Verification | `web/src/routes/verify-2fa/+page.svelte` | TOTP verification page |

**Auth Store Usage:**

```typescript
import { appAuthStore } from '@chendingplano/shared';

// Login
const result = await appAuthStore.login(email, password);
if (result.status === '2fa_required') {
  goto('/verify-2fa');
} else if (result.status === 'success') {
  goto(result.redirect_url || '/dashboard');
}

// Register
await appAuthStore.register({
  email,
  password,
  passwordConfirm,
  first_name,
  last_name
});

// Check auth status
await appAuthStore.checkAuthStatus();

// Logout
await appAuthStore.logout();
```

**Auth Store State:**

```typescript
// Reactive state (Svelte 5 runes)
appAuthStore.isLoggedIn  // boolean
appAuthStore.user        // User object or null
appAuthStore.isAdmin     // boolean
appAuthStore.isOwner     // boolean
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_USE_KRATOS` | `"false"` | Set to `"true"` to enable Kratos authentication |
| `KRATOS_PUBLIC_URL` | `http://localhost:4433` | Kratos public API URL |
| `KRATOS_ADMIN_URL` | `http://localhost:4434` | Kratos admin API URL |
| `VITE_DEV_ONLY_URL` | - | URL for Vite, applicable in dev environment only, used by Go|
| `VITE_DEV_ONLY_URL` | `/dashboard` | Default redirect after login |
| `GOOGLE_OAUTH_CLIENT_ID` | - | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | - | Google OAuth client secret |
| `GOOGLE_OAUTH_REDIRECT_URL` | - | Google OAuth callback URL |

### Kratos Configuration

**File:** `Ory/kratos/kratos.yml`

**Identity Schema:** `Ory/kratos/identity.schema.json`

```json
{
  "$id": "https://example.com/identity.schema.json",
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Identity",
  "type": "object",
  "properties": {
    "traits": {
      "type": "object",
      "properties": {
        "email": {
          "type": "string",
          "format": "email",
          "title": "Email",
          "ory.sh/kratos": {
            "credentials": {
              "password": { "identifier": true }
            },
            "verification": { "via": "email" },
            "recovery": { "via": "email" }
          }
        },
        "name": {
          "type": "object",
          "properties": {
            "first": { "type": "string", "title": "First Name" },
            "last": { "type": "string", "title": "Last Name" }
          },
          "required": ["first", "last"]
        }
      },
      "required": ["email", "name"]
    }
  }
}
```

**Enabled Authentication Methods:**

```yaml
selfservice:
  methods:
    password:
      enabled: true
    oidc:
      enabled: true
    totp:
      enabled: true
    lookup_secret:
      enabled: true
    link:
      enabled: true
    code:
      enabled: true
```

For complete configuration, see [Configuration Guide](./config.md).

---

## Security Features

### Rate Limiting

Login attempts are rate-limited per IP address and per account to prevent brute-force attacks.

**Implementation:** `shared/go/api/auth/rate_limiter.go`

### CSRF Protection

All authentication endpoints include CSRF token validation.

**Implementation:** `shared/go/api/auth/csrf.go`

### Password Validation

Passwords must meet strength requirements:

- Minimum length
- Complexity rules (uppercase, lowercase, numbers, special characters)
- Not in common password lists

**Implementation:** `shared/go/api/auth/password_validation.go`

### Two-Factor Authentication (2FA)

TOTP-based 2FA is supported via Kratos.

**Flow:**

1. User enables 2FA in settings
2. Kratos generates TOTP secret
3. User scans QR code with authenticator app
4. On login, user is prompted for TOTP code
5. Session is upgraded from AAL1 to AAL2

### Session Management

- Session cookies: `ory_kratos_session` (browser) or `session_token` (API)
- Default session duration: 24 hours (configurable)
- Secure cookie settings for production

### Activity Logging

All authentication events are logged for audit purposes.

---

## API Reference

### Public Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/email/login` | Login with email/password |
| POST | `/auth/email/signup` | Register with email/password |
| POST | `/auth/email/forgot` | Initiate password reset |
| GET | `/auth/google/login` | Initiate Google OAuth |
| GET | `/auth/github/login` | Initiate GitHub OAuth |
| GET | `/oauth/callback` | OAuth callback handler |
| POST | `/auth/verify-2fa` | Verify TOTP code |
| DELETE | `/auth/logout` | Logout and clear session |
| GET | `/auth/me` | Get current user session |

### Protected Endpoints

All `/api/v1/*` endpoints require authentication via `AuthMiddleware`.

---

## Related Documentation

- [Configuration Guide](./config.md) - Environment and service configuration
- [Ory Kratos Documentation](../Ory/CLAUDE.md) - Kratos setup and customization
- [Ory Official Docs](https://www.ory.com/docs/kratos) - Complete Kratos documentation

---

## Troubleshooting

### Common Issues

**1. Session Not Persisting**

- Ensure you're accessing via `127.0.0.1` not `localhost` (cookie domain matching)
- Check `allow_credentials: true` in CORS config

**2. OAuth Redirect Errors**

- Verify `default_browser_return_url` in `kratos.yml` matches `APP_BASE_URL`
- Ensure callback URLs are registered in Google Cloud Console

**3. 401 Unauthorized Errors**

- Check `KRATOS_PUBLIC_URL` points to running Kratos instance
- Verify `AUTH_USE_KRATOS="true"` is set
- Ensure session cookie is being sent with requests

**4. Email Verification Not Working**

- Verify `RESEND_API_KEY` is set correctly
- Check sender domain is verified in Resend account
- Check spam folder for verification emails

See [Configuration Guide](./config.md) for complete troubleshooting steps.
