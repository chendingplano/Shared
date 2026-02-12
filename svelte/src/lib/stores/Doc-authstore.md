# 1 Integrate Svelte Auth Store with Ory Kratos

## 1.1 Overview
Integrate shared/svelte/src/lib/stores/auth.svelte.ts with Ory Kratos using a backend proxy approach. The existing /auth/* endpoints remain, but the Go backend validates sessions via Kratos API. Frontend changes are minimal.

## 1.2 Architecture
```text
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│  Svelte Frontend    │────▶│  Go Backend         │────▶│  Ory Kratos         │
│  (auth.svelte.ts)   │     │  (existing routes)  │     │  (port 4433)        │
│                     │     │                     │     │                     │
│  /auth/me           │     │  Validates session  │     │  /sessions/whoami   │
│  /auth/email/login  │     │  via Kratos API     │     │  /self-service/*    │
│  /auth/logout       │     │                     │     │                     │
└─────────────────────┘     └─────────────────────┘     └─────────────────────┘
```

## 1.3 Files to Modify
### 1.3.1 Svelte Auth Store
File: ```text shared/svelte/src/lib/stores/auth.svelte.ts```

Changes:

- Update checkAuthStatus() to handle Kratos session response format
- Update login() to work with Kratos login flow response
- Update logout() to trigger Kratos logout flow
- Keep register() but adapt for Kratos registration flow
- Update/simplify verifyEmail() (Kratos handles verification differently)
- Add Kratos-specific session/identity types

### 1.3.2 UserInfo Type Mapping
File: ```text shared/svelte/src/lib/types/CommonTypes.ts```

Map Kratos identity traits to existing UserInfo:

```text
// Kratos identity.traits:
{
  email: "user@example.com",
  name: { first: "John", last: "Doe" }
}

// Maps to UserInfo:
{
  id: identity.id,
  email: traits.email,
  first_name: traits.name.first,
  last_name: traits.name.last,
  user_status: session.active ? "active" : "inactive",
  admin: false,  // Set by backend based on identity metadata
  is_owner: false
}
```

## 1.3 Implementation Steps

### 1.3.1 Step 1: Add Kratos Types
Add types for Kratos session/identity response in auth.svelte.ts:

```text
interface KratosSession {
  id: string;
  active: boolean;
  expires_at: string;
  authenticated_at: string;
  identity: KratosIdentity;
}

interface KratosIdentity {
  id: string;
  traits: {
    email: string;
    name?: { first?: string; last?: string };
  };
  metadata_public?: {
    admin?: boolean;
    is_owner?: boolean;
  };
  created_at: string;
  updated_at: string;
}
```

### 1.3.2 Step 2: Update checkAuthStatus()
The /auth/me endpoint (Go backend) will now return Kratos session data. Update the handler:

```text
const checkAuthStatus = async () => {
  // ... existing early return logic ...

  try {
    const response = await fetch('/auth/me', {
      method: 'GET',
      credentials: 'include',
    });

    if (response.ok) {
      const data = await response.json();
      // Backend returns: { session: KratosSession, base_url: string }
      const session = data.session as KratosSession;
      const identity = session.identity;
      const traits = identity.traits;

      const userInfo: UserInfo = {
        id: identity.id,
        email: traits.email,
        first_name: traits.name?.first ?? '',
        last_name: traits.name?.last ?? '',
        user_status: session.active ? 'active' : 'inactive',
        admin: identity.metadata_public?.admin ?? false,
        is_owner: identity.metadata_public?.is_owner ?? false,
      };

      update(() => ({
        isLoggedIn: session.active,
        status: 'success',
        error_msg: '',
        user: userInfo,
        isAdmin: userInfo.admin ?? false,
        isOwner: userInfo.is_owner ?? false,
        baseURL: data.base_url,
        // ... impersonation state ...
      }));
    }
    // ... rest unchanged ...
  }
};
```

### 1.3.3 Step 3: Update login()
Keep the existing /auth/email/login endpoint. The Go backend will:

- Create a Kratos login flow
- Submit credentials to Kratos
- Return session cookie + user info

```typescript
async function login(email: string, password: string): Promise<LoginResults> {
  try {
    const response = await fetch('/auth/email/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include', // Important: receive Kratos session cookie
    });

    if (!response.ok) {
      // Handle Kratos error messages (password policy, account locked, etc.)
      const errorData = await response.json();
      return {
        status: false,
        error_msg: errorData.message || 'Login failed',
        redirect_url: '',
        LOC: 'SHD_ATH_LOGIN'
      };
    }

    const data = await response.json();
    // Backend returns: { session: KratosSession, redirect_url: string }
    const session = data.session;
    const userInfo = mapKratosIdentityToUserInfo(session.identity);

    update((current) => ({
      ...current,
      user: userInfo,
      isLoggedIn: true,
      isAdmin: userInfo.admin ?? false,
      isOwner: userInfo.is_owner ?? false,
      status: 'success',
    }));

    return {
      status: true,
      error_msg: '',
      redirect_url: data.redirect_url || '/dashboard',
      LOC: ''
    };
  } catch (error) {
    // ... error handling ...
  }
}
```

### 1.3 4 Step 4: Update logout()
Kratos logout requires a flow. The Go backend handles this:

```typescript
async function logout(): Promise<void> {
  try {
    const response = await fetch('/auth/logout', {
      method: 'POST',
      credentials: 'include',
    });

    // Backend creates logout flow and returns logout_url or handles it server-side
    const data = await response.json();

    clearPersistedImpersonation();
    update(current => ({
      ...current,
      status: 'logout',
      user: null,
      isLoggedIn: false,
      isAdmin: false,
      isOwner: false,
      isImpersonating: false,
      impersonatedClientId: null,
      impersonatedClientName: null,
    }));

    // Redirect to login or Kratos logout URL
    if (typeof window !== 'undefined') {
      window.location.href = data.redirect_url || '/login';
    }
  } catch (error) {
    console.error('Logout failed:', error);
  }
}
```

### 1.3 5 Step 5: Update register()
Kratos registration flow is similar. Backend handles flow creation:

```typescript
async function register(userData: {...}): Promise<void> {
  try {
    const { email, password, first_name, last_name } = userData;

    if (userData.password !== userData.passwordConfirm) {
      // ... validation error ...
      return;
    }

    const res = await fetch('/auth/email/signup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        traits: {
          email,
          name: { first: first_name, last: last_name }
        },
        password
      }),
      credentials: 'include',
    });

    if (res.ok) {
      const data = await res.json();
      // Kratos may auto-login after registration (configured via hooks)
      if (data.session) {
        // User is logged in automatically
        const userInfo = mapKratosIdentityToUserInfo(data.session.identity);
        update(current => ({
          ...current,
          user: userInfo,
          isLoggedIn: true,
          status: 'success',
        }));
      } else {
        // Verification email sent
        alert('Check your email to verify your account.');
        update(current => ({ ...current, status: 'signup' }));
      }
    } else {
      const errorData = await res.json();
      alert('Registration failed: ' + errorData.message);
    }
  } catch (error) {
    // ... error handling ...
  }
}
```

### 1.3.6 Step 6: Simplify verifyEmail()
Kratos handles email verification via its own flow. The frontend just needs to check if verification succeeded:

```typescript
async function verifyEmail(token: string, _loc: string): Promise<VerifyEmailResults> {
  try {
    // Kratos verification is handled via redirect flow
    // This endpoint checks if the current session is verified
    const response = await fetch(`/auth/verify-status?token=${token}`, {
      method: 'GET',
      credentials: 'include',
    });

    if (response.ok) {
      const data = await response.json();
      if (data.verified) {
        // Re-fetch session to get updated state
        await checkAuthStatus();
        return {
          status: true,
          error_msg: '',
          redirect_url: data.redirect_url || '/dashboard',
          LOC: 'SHD_ATH_VERIFY'
        };
      }
    }

    return {
      status: false,
      error_msg: 'Verification failed',
      redirect_url: '/login',
      LOC: 'SHD_ATH_VERIFY_FAIL'
    };
  } catch (error) {
    // ... error handling ...
  }
}
```

### 1.3.7 Step 7: Add Helper Function

```typescript
function mapKratosIdentityToUserInfo(identity: KratosIdentity): UserInfo {
  const traits = identity.traits;
  const metadata = identity.metadata_public || {};

  return {
    id: identity.id,
    email: traits.email,
    first_name: traits.name?.first ?? '',
    last_name: traits.name?.last ?? '',
    name: traits.name ? `${traits.name.first ?? ''} ${traits.name.last ?? ''}`.trim() : '',
    user_status: 'active',
    admin: metadata.admin ?? false,
    is_owner: metadata.is_owner ?? false,
    verified: true, // If we have a session, email is verified (per Kratos config)
    auth_type: 'kratos',
  };
}
```

# 1.4 Backend Requirements (Go)
The Go backend needs to implement these endpoints using Ory Kratos SDK:

| Endpoint	| Kratos Integration |
|-----------|--------------------|
| GET /auth/me	| Call kratosClient.FrontendAPI.ToSession() with cookies |
| POST /auth/email/login	| Create login flow, submit credentials, return session |
| POST /auth/email/signup	| Create registration flow, submit traits + password |
| POST /auth/logout	| Create logout flow, invalidate session |
| GET /auth/verify-status	| Check identity verification status |

Example backend pattern (from Ory/backend/main.go):

```go
session, _, err := kratosClient.FrontendAPI.ToSession(ctx).
  Cookie(cookieHeader).
  Execute()
```

# 1.5 Impersonation Feature
The impersonation feature is frontend-only (sessionStorage) and doesn't need changes. It still works the same way - admin selects a client, and the frontend tracks which client is being viewed.

# 1.6 Verification Steps
- Start Kratos: cd ~/Workspace/Ory && mise start-kratos
- Start backend: Ensure Go backend proxies to Kratos
- Test login: Enter credentials in Svelte login form
- Test session: Refresh page, verify checkAuthStatus() works
- Test logout: Click logout, verify session cleared
- Test registration: Create new account, verify email flow works

# 1.7 Files Changed Summary
| File	| Changes |
|-------|---------|
| shared/svelte/src/lib/stores/auth.svelte.ts	| Update all auth methods for Kratos response format |
| shared/svelte/src/lib/types/CommonTypes.ts	| Add Kratos types (optional, can be inline) |

# 1.8 Notes
- The Go backend auth handlers need to be updated separately to proxy to Kratos
- Session cookies are set by Kratos and passed through the backend
- Admin/owner status should be stored in Kratos identity metadata (not traits)
- The baseURL field should come from backend config, not Kratos

# 1.9 Changes Made to auth.svelte.ts
## 1.9.1. Added Kratos Types (lines 23-74)
- KratosIdentity - Kratos identity with traits and metadata
- KratosSession - Kratos session structure
- AuthMeResponse, LoginResponse, LogoutResponse, SignupResponse, VerifyStatusResponse - Backend response types

## 1.9.2. Added Helper Function (lines 137-152)
- mapKratosIdentityToUserInfo() - Converts Kratos identity traits to the existing UserInfo type

## 1.9.3. Updated checkAuthStatus() (lines 231-320)
- Now expects { session: KratosSession, base_url: string } from /auth/me
- Uses mapKratosIdentityToUserInfo() to convert identity

## 1.9.4. Updated login() (lines 333-392)
- Added credentials: 'include' for Kratos session cookies
- Expects { session: KratosSession, redirect_url: string } from backend
- Removed the alert() call

## 1.9.5. Updated logout() (lines 394-451)
- Simplified to just call /auth/logout with credentials
- Handles redirect URL from backend response

## 1.9.6. Updated register() (lines 453-545)
- Sends data in Kratos traits format: { traits: { email, name: { first, last } }, password }
- Handles auto-login case when Kratos session hook is enabled
- Removed storing password in state (security fix)

## 1.9.7. Simplified verifyEmail() (lines 547-595)
- Changed endpoint to /auth/verify-status?token=...
- Re-fetches session via checkAuthStatus() after successful verification

# 1.10 Backend Requirements
Your Go backend needs to implement these endpoints to proxy to Kratos:

| Endpoint	| Backend Action |
|-----------|----------------|
| GET /auth/me	| Call kratosClient.FrontendAPI.ToSession() → return { session, base_url } |
| POST /auth/email/login	| Create login flow, submit credentials → return { session, redirect_url } |
| POST /auth/email/signup	| Create registration flow → return { session?, message?, redirect_url? } |
| POST /auth/logout	| Create logout flow → return { redirect_url } |
| GET /auth/verify-status	| Check verification status → return { verified, redirect_url? } |

# 2. Current Implementation
## 2.1 TypeScript Files

**File**: ```text auth.svelte.ts (/Users/cding/Workspace/shared/svelte/src/lib/stores/auth.svelte.ts) ``` \
**Description**: Svelte reactive auth store that manages frontend authentication state. Provides login/logout/register/verifyEmail functions, checks session status via `/auth/me`, and maps Kratos identity traits to the app's `UserInfo` type. Includes admin impersonation feature with sessionStorage persistence.

**File**: ```text ory.ts (/Users/cding/Workspace/ChenWeb/web/src/lib/services/ory.ts) ``` \
**Description**: TypeScript client for direct Ory Kratos API interaction. Provides functions for creating and submitting login/registration/recovery flows, session management (`getSession`, `logout`), Google OAuth support via OIDC, and helper utilities for CSRF token extraction and error message parsing.

## 2.2 Backend Files
**File**: ```text router.go (/Users/cding/Workspace/shared/go/api/router.go) ``` \
**Description**: Go router that registers all `/auth/*` endpoints. Conditionally routes to Kratos or legacy handlers based on `AUTH_USE_KRATOS` env variable. Registers OAuth (Google, GitHub), email login/signup, verification, and password reset routes. Also registers CSRF middleware and shared API routes.

**File**: ```text kratos.go (/Users/cding/Workspace/shared/go/api/auth/kratos.go) ``` \
**Description**: Go backend implementation for Ory Kratos integration using the Ory SDK. Provides
- `HandleEmailLoginKratos` (creates native login flow, submits credentials, sets session cookie), 
- `HandleEmailSignupKratos` (creates native registration flow, submits registration, sets session cookie), 
- `HandleAuthMeKratos` (validates session via `ToSession()`, returns user info), 
- `HandleLogoutKratos` (creates/submits logout flow), and 
- `KratosAuthMiddleware` for protected routes. Includes security features: IP-based rate limiting, per-account lockout, and CSRF origin validation.


# 3. Setup
## 3.1 Environment Variables

| Name | Default | Explanation |
|------|---------|----------------|
| AUTH_USE_KRATOS	| true | Use Kratos if it is set to true|
| KRATOS_PUBLIC_URL | http://127.0.0.1:4433 | The URL to access Kratos |

## 3.2 Response Format 

The /auth/email/login endpoint now returns the format expected by the frontend:

```json
{
  "status": "ok",
  "redirect_url": "/dashboard",
  "session": {
    "id": "session-id",
    "active": true,
    "expires_at": "...",
    "identity": {
      "id": "identity-id",
      "traits": { "email": "...", "name": { "first": "...", "last": "..." } },
      "metadata_public": { "admin": false, "is_owner": false }
    }
  }
}
```