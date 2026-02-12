# Auth Store Documentation

**File:** `shared/svelte/src/lib/stores/auth.svelte.ts`

A Svelte store that manages authentication state, integrating with Ory Kratos for identity management.

## Overview

The `appAuthStore` is a singleton Svelte store that handles:

- User authentication (login, logout, registration)
- Session management via Kratos cookies
- Email verification
- Client impersonation (for admin users)
- Role-based access (admin, owner flags)

## Quick Start

```typescript
import { appAuthStore } from '$lib/stores/auth.svelte.ts';

// Wait for auth check to complete
await appAuthStore.ready();

// Check if logged in
if (appAuthStore.getIsLoggedIn()) {
  const user = appAuthStore.getUser();
  console.log('Logged in as:', user?.email);
}
```

## State Shape

```typescript
interface AuthStoreState {
  isLoggedIn: boolean;
  status: string;           // 'login' | 'success' | 'error' | 'logout' | 'signup'
  error_msg: string;
  user: UserInfo | null;
  isAdmin: boolean;
  isOwner: boolean;
  baseURL: string;
  isImpersonating: boolean;
  impersonatedClientId: string | null;
  impersonatedClientName: string | null;
}
```

## API Reference

### Authentication Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `login` | `(email: string, password: string) => Promise<LoginResults>` | Authenticate user via email/password |
| `logout` | `() => Promise<void>` | End session and redirect to `/login` |
| `register` | `(userData: RegisterData) => Promise<void>` | Create new account |
| `verifyEmail` | `(token: string, loc: string) => Promise<VerifyEmailResults>` | Verify email address |
| `checkAuthStatus` | `() => Promise<void>` | Refresh auth state from server |

### State Accessors

| Method | Return Type | Description |
|--------|-------------|-------------|
| `getIsLoggedIn()` | `boolean` | Check if user is authenticated |
| `getUser()` | `UserInfo \| null` | Get current user info |
| `getIsAdmin()` | `boolean` | Check admin role |
| `getIsOwner()` | `boolean` | Check owner role |
| `getBaseURL()` | `string` | Get API base URL |
| `ready()` | `Promise<void>` | Resolves when initial auth check completes |

### Impersonation Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `startImpersonation` | `(clientId: string, clientName: string) => void` | Begin impersonating a client |
| `stopImpersonation` | `() => void` | End impersonation |
| `getIsImpersonating()` | `boolean` | Check if currently impersonating |
| `getImpersonatedClientId()` | `string \| null` | Get impersonated client ID |
| `getImpersonatedClientName()` | `string \| null` | Get impersonated client name |

### Svelte Store Methods

| Method | Description |
|--------|-------------|
| `subscribe` | Standard Svelte store subscription |
| `setUserInfo` | Manually set user info (use with caution) |

## Usage Examples

### Login Flow

```typescript
const result = await appAuthStore.login('user@example.com', 'password');

if (result.status) {
  // Success - user is redirected automatically
  console.log('Redirecting to:', result.redirect_url);
} else {
  // Handle error
  console.error('Login failed:', result.error_msg);
}
```

### Registration

```typescript
await appAuthStore.register({
  user_id: '',
  email: 'new@example.com',
  password: 'securePassword',
  passwordConfirm: 'securePassword',
  first_name: 'John',
  last_name: 'Doe',
  is_admin: false
});
```

### Reactive Subscription (Svelte Component)

```svelte
<script>
  import { appAuthStore } from '$lib/stores/auth.svelte.ts';

  let user = $state(null);

  $effect(() => {
    const unsubscribe = appAuthStore.subscribe(state => {
      user = state.user;
    });
    return unsubscribe;
  });
</script>

{#if user}
  <p>Welcome, {user.first_name}!</p>
{:else}
  <p>Please log in</p>
{/if}
```

### Impersonation (Admin Feature)

```typescript
// Start impersonating a client
appAuthStore.startImpersonation('client-id-123', 'John Doe');

// Check impersonation status
if (appAuthStore.getIsImpersonating()) {
  const clientId = appAuthStore.getImpersonatedClientId();
  const clientName = appAuthStore.getImpersonatedClientName();
  console.log(`Impersonating: ${clientName} (${clientId})`);
}

// Stop impersonating
appAuthStore.stopImpersonation();
```

## Backend Endpoints

The store communicates with these backend endpoints:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/auth/me` | GET | Check current session |
| `/auth/email/login` | POST | Authenticate with credentials |
| `/auth/email/signup` | POST | Register new account |
| `/auth/logout` | POST | End session |
| `/auth/verify-status` | GET | Check email verification |

## Kratos Integration

The store integrates with Ory Kratos for identity management:

- **Session Cookies**: All requests include `credentials: 'include'` for cookie-based sessions
- **Identity Mapping**: Kratos identities are mapped to `UserInfo` via `mapKratosIdentityToUserInfo()`
- **Traits**: User data stored in Kratos traits (email, name)
- **Metadata**: Role flags (admin, is_owner) stored in `metadata_public`

### Identity Structure

```typescript
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
}
```

## Persistence

### Session State
- Managed by Kratos via HTTP-only cookies
- Automatically validated on page load via `checkAuthStatus()`

### Impersonation State
- Persisted in `sessionStorage` (tab-scoped)
- Cleared on logout or tab close
- Restored on page refresh within same tab

## Initialization Behavior

On store creation:

1. **SSR Context**: `ready()` resolves immediately (no auth check possible)
2. **Browser Context**: Calls `/auth/me` to check existing session
3. **State Update**: Updates store with session data or clears to logged-out state
4. **Ready Resolution**: `ready()` promise resolves after check completes

```typescript
// Always wait for ready() before relying on auth state
await appAuthStore.ready();
const isLoggedIn = appAuthStore.getIsLoggedIn();
```

## Error Handling

Errors include location codes for debugging:

| Code | Location |
|------|----------|
| `SHD_ATH_185` | Missing base_url in /auth/me response |
| `SHD_ATH_120` | Auth check failed |
| `SHD_ATH_LOGIN` | Login request failed |
| `SHD_ATH_LOGIN_ERR` | Login exception |
| `SHD_ATH_VERIFY_FAIL` | Email verification failed |
| `SHD_ATH_VERIFY_OK` | Email verification succeeded |
| `SHD_ATH_VERIFY_PENDING` | Email verification pending |
| `SHD_ATH_VERIFY_ERR` | Email verification exception |
| `SHD_0207182800` | User not logged in |

## Navigation Hook

The store exports a navigation callback for host app integration:

```typescript
import { onNavigate } from '$lib/stores/auth.svelte.ts';

// Set custom navigation handler
onNavigate = (path: string) => {
  goto(path); // SvelteKit navigation
};
```

## Type Exports

```typescript
export type LoginResults = {
  status: boolean;
  error_msg: string;
  redirect_url: string;
  LOC: string;
};

export type VerifyEmailResults = {
  status: boolean;
  error_msg: string;
  redirect_url: string;
  LOC: string;
};
```

## HMR Support

The store handles Hot Module Replacement gracefully by cleaning up previous subscriptions on recreation:

```typescript
if (internalUnsubscribe) {
  internalUnsubscribe();
  internalUnsubscribe = null;
}
```
