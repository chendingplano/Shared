# Security and Access Control

This document describes the authentication and access control architecture for web applications in this workspace, using Ory Kratos for identity management.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Browser                                        │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│  Page Request │    │  API Request  │    │  Auth Request │
│  /dashboard   │    │  /api/*       │    │  /auth/*      │
└───────┬───────┘    └───────┬───────┘    └───────┬───────┘
        │                    │                    │
        ▼                    │                    │
┌───────────────────┐        │                    │
│ SvelteKit Server  │        │                    │
│ hooks.server.ts   │        │                    │
│ (Route Protection)│        │                    │
└───────┬───────────┘        │                    │
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
                             ▼
                  ┌────────────────────-─┐
                  │    Go Backend        │
                  │    (Port 8080)       │
                  │                      │
                  │  - /auth/me          │
                  │  - /auth/login       │
                  │  - /auth/logout      │
                  │  - /api/* (protected)│
                  └──────────┬─────────-─┘
                             │
                             ▼
                  ┌─────────────────────┐
                  │   Ory Kratos        │
                  │   (Port 4433)       │
                  │                     │
                  │  - Session mgmt     │
                  │  - Identity mgmt    │
                  │  - Password hashing │
                  └─────────────────────┘
```

## Two Layers of Protection

### Layer 1: Frontend Route Protection (SvelteKit)

**Purpose:** Prevent unauthenticated users from even loading protected pages.

**Location:** `web/src/hooks.server.ts`

```typescript
// Routes that require authentication
const PROTECTED_ROUTES = ['/dashboard', '/settings', '/admin', ...]; // Make sure you put all protected here

const handleAuth: Handle = async ({ event, resolve }) => {
    const pathname = event.url.pathname;

    if (isProtectedRoute(pathname)) {
        const cookies = event.request.headers.get('cookie');
        const isAuthenticated = await validateSession(cookies);

        if (!isAuthenticated) {
            const returnTo = encodeURIComponent(event.url.pathname + event.url.search);
            throw redirect(302, `/Ory/login?return_to=${returnTo}`);
        }
    }

    return resolve(event);
};
```

**How it works:**
1. Request arrives at SvelteKit server
2. `hooks.server.ts` intercepts BEFORE page renders (still on the server side)
3. Checks if route is in `PROTECTED_ROUTES`
4. Validates session by calling Go backend `/auth/me`
5. If invalid → redirect to login (page never loads)
6. If valid → continue to render page

**Key point:** This runs server-side. Unauthenticated users never receive the page HTML.

### Layer 2: Backend API Protection (Go)

**Purpose:** Protect data endpoints regardless of how they're called.

**Location:** Go backend middleware

The Go backend protects `/api/*` routes using middleware that:
1. Extracts session cookie from request
2. Validates with Kratos `/sessions/whoami`
3. Returns 401 Unauthorized if invalid

**Why both layers?**

| Scenario | Layer 1 (Frontend) | Layer 2 (Backend) |
|----------|-------------------|-------------------|
| User visits `/dashboard` without login | Redirects to login | N/A (no API call yet) |
| Script calls `/api/data` directly | N/A (not a page) | Returns 401 |
| Logged-in user visits `/dashboard` | Allows page load | Returns data |
| Token expires mid-session | Next page nav redirects | API calls return 401 |

## Session Management

### How Sessions Work

1. **Login:** User submits credentials → Go backend → Kratos validates → Session cookie set
2. **Session Cookie:** `ory_kratos_session` cookie stored in browser
3. **Validation:** Every protected request validates cookie with Kratos
4. **Logout:** Session invalidated in Kratos, cookie cleared

### Session Flow Diagram

```
Login:
  Browser → POST /auth/email/login → Go Backend → Kratos
                                          │
                                          ▼
                               Set-Cookie: ory_kratos_session=...
                                          │
                                          ▼
                                       Browser stores cookie

Subsequent Requests:
  Browser (with cookie) → /dashboard → hooks.server.ts
                                            │
                                            ▼
                                   GET /auth/me (with cookie)
                                            │
                                            ▼
                                   Go Backend → Kratos /sessions/whoami
                                            │
                                    ┌───────┴───────┐
                                    ▼               ▼
                               200 OK          401 Unauthorized
                                    │               │
                                    ▼               ▼
                              Render page     Redirect to login
```

## Client-Side Auth Store

**Location:** `shared/svelte/src/lib/stores/auth.svelte.ts`

The `appAuthStore` provides client-side auth state management:

```typescript
import { appAuthStore } from '@chendingplano/shared';

// Check if logged in
if (appAuthStore.getIsLoggedIn()) {
    const user = appAuthStore.getUser();
    console.log('Logged in as:', user?.email);
}

// Subscribe to auth state changes
appAuthStore.subscribe(state => {
    if (!state.isLoggedIn) {
        // Handle logout
    }
});

// Login
const result = await appAuthStore.login(email, password);

// Logout
await appAuthStore.logout();
```

### Store Features

- **Auto-initialization:** Checks `/auth/me` on load
- **Session state:** `isLoggedIn`, `user`, `isAdmin`, `isOwner`
- **Impersonation:** Admin can impersonate clients
- **Ready promise:** `await appAuthStore.ready()` to wait for initial auth check

### Client vs Server Auth Check

| Aspect | Client (`appAuthStore`) | Server (`hooks.server.ts`) |
|--------|------------------------|---------------------------|
| Runs in | Browser | SvelteKit server |
| Purpose | UI state, conditional rendering | Route protection |
| Timing | After page loads | Before page renders |
| Security | UX convenience | Actual protection |

**Important:** Client-side checks are for UX (showing/hiding UI). Server-side checks are for security (preventing unauthorized access).

## Configuration

### Environment Variables

```bash
# Go Backend URL (for SvelteKit server-side calls)
API_BASE_URL=http://localhost:8080

# Kratos URL (for direct Kratos calls if needed)
KRATOS_PUBLIC_URL=http://127.0.0.1:4433
```

### Adding Protected Routes

Edit `hooks.server.ts`:

```typescript
const PROTECTED_ROUTES = [
    '/dashboard',
    '/settings',
    '/admin',
    '/reports',
    // Add more routes here
];
```

Routes match by prefix, so `/dashboard` protects:
- `/dashboard`
- `/dashboard/reports`
- `/dashboard/settings/profile`

### Login Redirect URL

When redirecting to login, the original URL is preserved:

```typescript
throw redirect(302, `/login?return_to=${returnTo}`);
```

The login page should redirect back after successful authentication:

```typescript
// In login success handler
const returnTo = new URLSearchParams(window.location.search).get('return_to');
window.location.href = returnTo || '/dashboard';
```

## Security Best Practices

### Do

- Always protect routes server-side via `hooks.server.ts`
= Periodically review routes in `hooks.server.ts`
- Validate sessions on every API request in the backend
- Use HTTPS in production (cookies are HttpOnly, Secure)
- Set appropriate session lifetimes in Kratos config

### Don't

- Rely solely on client-side auth checks for security
- Store sensitive data in localStorage/sessionStorage
- Skip backend validation assuming frontend will protect
- Expose Kratos admin API (port 4434) publicly

## Troubleshooting

### "Page loads but shows no data"

- Frontend route protection may be missing
- Add route to `PROTECTED_ROUTES` in `hooks.server.ts`

### "Session not persisting"

- Ensure cookies are sent with `credentials: 'include'`
- Check cookie domain matches (use `127.0.0.1` not `localhost` for Kratos)
- Verify Kratos session lifespan in `kratos.yml`

### "Redirect loop on login page"

- Ensure login route (`/Ory/login`) is NOT in `PROTECTED_ROUTES`
- Check that auth validation doesn't fail for non-protected routes

### "401 on API calls after login"

- Cookie may not be forwarded (check `credentials: 'include'`)
- Session may have expired (check Kratos session lifespan)
- CORS may be blocking cookies (check backend CORS config)

## Related Documentation

- [Ory Kratos Setup](../Ory/CLAUDE.md) - Kratos configuration and development
- [Auth Store API](../svelte/src/lib/stores/auth.svelte.ts) - Client-side auth store
- [Go Backend Auth](../ChenWeb/server/) - Backend auth middleware
