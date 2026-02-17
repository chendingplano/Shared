/*******************************************************
 * How to use impersonate feature:
import { appAuthStore } from '$lib/stores/auth.svelte.ts';

// Start impersonating a client
appAuthStore.startImpersonation('client-id-123');

// Check if impersonating
if (appAuthStore.getIsImpersonating()) {
  const clientId = appAuthStore.getImpersonatedClientId();
  console.log('Impersonating client:', clientId);
}

// Stop impersonating
appAuthStore.stopImpersonation();
********************************************************/

import { writable } from 'svelte/store';
import type { UserInfo, FileNameString } from '../types/CommonTypes';

export let onNavigate = (path: string) => {}; // passed from the host app

// Kratos session/identity types (from Ory Kratos API)
interface KratosIdentity {
  id: string;
  traits: {
    email: string;
    name?: { first?: string; last?: string };
  };
  metadata_public?: {
    admin?: boolean;
    is_owner?: boolean;
    avatar?: string;
  };
  state?: string; // "active" | "inactive"
  created_at: string;
  updated_at: string;
}

interface KratosSession {
  id: string;
  active: boolean;
  expires_at: string;
  authenticated_at: string;
  identity: KratosIdentity;
}

// Response format from backend /auth/me endpoint
interface AuthMeResponse {
  session: KratosSession;
  base_url: string;
}

// Response format from backend /auth/email/login endpoint
interface LoginResponse {
  session: KratosSession;
  redirect_url: string;
}

// Response format from backend /auth/logout endpoint
interface LogoutResponse {
  redirect_url: string;
}

// Response format from backend /auth/email/signup endpoint
interface SignupResponse {
  session?: KratosSession;
  message?: string;
  redirect_url?: string;
}

// Response format from backend /auth/verify-status endpoint
interface VerifyStatusResponse {
  verified: boolean;
  redirect_url?: string;
}

interface AuthStoreState {
  isLoggedIn:       boolean;
  status:           string;
  error_msg:        string;
  user:             UserInfo | null;
  isAdmin:          boolean;
  isOwner:          boolean;
  baseURL:          string;
  isImpersonating:  boolean;
  impersonatedClientId: string | null;
  impersonatedClientName: string | null;
}

export type LoginResults = {
  status:           boolean
  error_msg:        string
  redirect_url:     string
  LOC:              string
}

export type VerifyEmailResults = {
  status:           boolean
  error_msg:        string
  redirect_url:     string
  LOC:              string
}

let internalUnsubscribe: (() => void) | null = null;

interface AuthStore {
  setUserInfo:      (userInfo: UserInfo | null) => void;
  login:            (email: string, password: string) => Promise<LoginResults>;
  logout:           () => void;
  register: (userData: {
    user_id?:         string;
    email:            string;
    password:         string;
    passwordConfirm:  string;
    first_name:       string;
    last_name:        string;
    is_admin:         boolean;
  }) => Promise<void>;
  verifyEmail:      (token: string, loc: string) => Promise<VerifyEmailResults>;
  checkAuthStatus:  () => Promise<void>;
  // Add methods to check authentication state
  subscribe:        (run: (value: AuthStoreState) => void) => () => void;
  getIsLoggedIn:    () => boolean;
  getUser:          () => UserInfo | null;
  getIsAdmin:       () => boolean;
  getIsOwner:       () => boolean;
  getBaseURL:       () => string;
  ready:            () => Promise<void>;
  // Impersonation methods
  startImpersonation: (clientId: string, clientName: string) => void;
  stopImpersonation:  () => void;
  getIsImpersonating: () => boolean;
  getImpersonatedClientId: () => string | null;
  getImpersonatedClientName: () => string | null;
}

// Helper function to map Kratos identity to UserInfo
function mapKratosIdentityToUserInfo(identity: KratosIdentity): UserInfo {
  const traits = identity.traits;
  const metadata = identity.metadata_public || {};

  return {
    id: identity.id,
    email: traits.email,
    first_name: traits.name?.first ?? '',
    last_name: traits.name?.last ?? '',
    name: traits.name ? `${traits.name.first ?? ''} ${traits.name.last ?? ''}`.trim() : undefined,
    user_status: identity.state ?? 'active',
    admin: metadata.admin ?? false,
    is_owner: metadata.is_owner ?? false,
    avatar: metadata.avatar as FileNameString | undefined,
    verified: true, // If we have a session, email is verified (per Kratos config)
    auth_type: 'kratos',
  };
}

function createAuthStore(): AuthStore {
    // 👇 CLEAN UP previous subscription if this is re-run (e.g., HMR)
    if (internalUnsubscribe) {
        internalUnsubscribe();
        internalUnsubscribe = null;
    }

    let readyResolve!: () => void;
    const readyPromise = new Promise<void>(resolve => {
        readyResolve = resolve;
    });

    // Cross-tab logout notification: when one tab logs out, all other tabs
    // clear auth state and redirect to login. This ensures stale tabs don't
    // hold open SSE connections or make requests with an invalid session.
    let authChannel: BroadcastChannel | null = null;
    if (typeof window !== 'undefined' && typeof BroadcastChannel !== 'undefined') {
        authChannel = new BroadcastChannel('mirai_auth');
        authChannel.onmessage = (event: MessageEvent) => {
            if (event.data?.type === 'logout') {
                clearPersistedImpersonation();
                update(() => ({
                    isLoggedIn: false,
                    status: 'login',
                    user: null,
                    error_msg: '',
                    isAdmin: false,
                    isOwner: false,
                    baseURL: '',
                    isImpersonating: false,
                    impersonatedClientId: null,
                    impersonatedClientName: null,
                }));
                window.location.href = '/login';
            }
        };
    }

    // Initialize the writable store with the state object
    const { subscribe, set, update } = writable<AuthStoreState>({
        isLoggedIn:   false,
        status:       'login',
        error_msg:    '',
        user:         null,
        isAdmin:      false,
        isOwner:      false,
        baseURL:      '',
        isImpersonating: false,
        impersonatedClientId: null,
        impersonatedClientName: null,
    });

    // Method to set user info
    const setUserInfo = (userInfo: UserInfo | null) => {
        update(currentState => ({
            ...currentState,
            user: userInfo,
            isLoggedIn: userInfo !== null && userInfo.user_status === 'active',
            isAdmin: userInfo?.admin?? false,
            isOwner: userInfo?.is_owner?? false,
        }));
    };

    // Helper functions to get current state values
    let currentState: AuthStoreState = {
        isLoggedIn:     false,
        status:         'login',
        error_msg:      '',
        user:           null,
        isAdmin:        false,
        isOwner:        false,
        baseURL:        '',
        isImpersonating: false,
        impersonatedClientId: null,
        impersonatedClientName: null,
    };

    // Subscribe to state changes to keep currentState updated
    internalUnsubscribe = subscribe(state => {
        currentState = state;
    });

    const getIsLoggedIn = (): boolean => {
        return currentState.isLoggedIn;
    };

    const getUser = (): UserInfo | null => {
        return currentState.user;
    };

    const getIsAdmin = (): boolean => {
        return currentState.isAdmin;
    };

    const getIsOwner = (): boolean => {
        return currentState.isOwner;
    };

    const getBaseURL = (): string => {
        return currentState.baseURL;
    };

    // Check if user is already logged in via Kratos session cookie
    const checkAuthStatus = async () => {
        // If already logged in with user info, skip the check
        if (currentState.isLoggedIn && currentState.user) {
            return;
        }

        // Abort if auth check takes longer than 5 seconds (prevents infinite loading
        // when the browser's connection pool is exhausted or the server is unreachable)
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 5000);

        try {
            const response = await fetch('/auth/me', {
                method: 'GET',
                credentials: 'include', // essential for Kratos session cookies
                signal: controller.signal,
            });

            if (response.ok) {
                const data = await response.json() as AuthMeResponse;
                const session = data.session;
                const base_url = data.base_url;

                if (!base_url) {
                    console.error('Missing base_url in /auth/me response (SHD_ATH_185)');
                }

                if (session && session.active) {
                    const userInfo = mapKratosIdentityToUserInfo(session.identity);

                    // Restore impersonation from sessionStorage if present
                    const persisted = getPersistedImpersonation();
                    update(() => ({
                        isLoggedIn: true,
                        status: 'success',
                        error_msg: '',
                        user: userInfo,
                        isAdmin: userInfo.admin ?? false,
                        isOwner: userInfo.is_owner ?? false,
                        baseURL: base_url ?? '',
                        isImpersonating: persisted !== null,
                        impersonatedClientId: persisted?.clientId ?? null,
                        impersonatedClientName: persisted?.clientName ?? null,
                    }));
                } else {
                    // Session exists but not active
                    clearPersistedImpersonation();
                    update(() => ({
                        isLoggedIn: false,
                        status: 'login',
                        user: null,
                        error_msg: '',
                        isAdmin: false,
                        isOwner: false,
                        baseURL: '',
                        isImpersonating: false,
                        impersonatedClientId: null,
                        impersonatedClientName: null,
                    }));
                }
            } else {
                // Not logged in — clear everything including persisted impersonation
                console.log(`user not logged in (SHD_0207182800)`)
                clearPersistedImpersonation();
                update(() => ({
                    isLoggedIn: false,
                    status: 'login',
                    user: null,
                    error_msg: '',
                    isAdmin: false,
                    isOwner: false,
                    baseURL: '',
                    isImpersonating: false,
                    impersonatedClientId: null,
                    impersonatedClientName: null,
                }));
            }
        } catch (error) {
            console.warn('Auth check failed (SHD_ATH_120):', error);
            clearPersistedImpersonation();
            update(() => ({
                isLoggedIn: false,
                status: 'login',
                user: null,
                error_msg: 'check login, exception occurred',
                isAdmin: false,
                isOwner: false,
                baseURL: '',
                isImpersonating: false,
                impersonatedClientId: null,
                impersonatedClientName: null,
            }));
        } finally {
            clearTimeout(timeoutId);
            // Resolve the ready promise regardless of success/failure
            readyResolve();
        }
    };

    // Auto-check auth status on store creation (browser only)
    if (typeof window === 'undefined') {
        // SSR: resolve immediately (no auth check possible)
        readyResolve();
    } else {
        // Browser: check auth status via /auth/me endpoint
        // This will call readyResolve() when complete
        checkAuthStatus();
    }

    async function login(email: string, password: string): Promise<LoginResults> {
        try {
            const response = await fetch('/auth/email/login', {
              method: 'POST',
              body: JSON.stringify({ email, password }),
              headers: { 'Content-Type': 'application/json' },
              credentials: 'include', // Important: receive Kratos session cookie
            });

            if (!response.ok) {
                const errorData = await response.json();
                const error_msg = errorData.message ?? response.statusText;
                return {
                  status: false,
                  error_msg: error_msg,
                  redirect_url: '',
                  LOC: 'SHD_ATH_LOGIN'
                };
            }

            // Backend returns: { session: KratosSession, redirect_url: string }
            const data = await response.json() as LoginResponse;
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

            const redirect_url = data.redirect_url || '/dashboard';
            window.location.href = redirect_url;
            return {
              status: true,
              error_msg: '',
              redirect_url: redirect_url,
              LOC: ''
            };
        } catch (error) {
            const error_msg = error instanceof Error ? error.message : 'Unknown error';
            console.error('Login error:', error_msg);

            update(current => ({
              ...current,
              status: 'error',
              user: null,
              isLoggedIn: false,
              isAdmin: false,
              isOwner: false,
            }));

            return {
              status: false,
              error_msg: error_msg,
              redirect_url: '',
              LOC: 'SHD_ATH_LOGIN_ERR'
            };
        }
    }

    async function logout(): Promise<void> {
        try {
            // Backend handles Kratos logout flow
            const response = await fetch('/auth/logout', {
              method: 'POST',
              credentials: 'include',
            });

            // Parse response for redirect URL (backend may return Kratos logout URL)
            let redirect_url = '/login';
            if (response.ok) {
                try {
                    const data = await response.json() as LogoutResponse;
                    redirect_url = data.redirect_url || '/login';
                } catch {
                    // Response may not be JSON, use default redirect
                }
            }

            // Clear local state
            if (typeof window !== 'undefined') {
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

              // Notify other tabs to clear auth state and disconnect SSE
              authChannel?.postMessage({ type: 'logout' });

              window.location.href = redirect_url;
            }
        } catch (error) {
            console.error('Logout process failed:', error instanceof Error ? error.message : 'Unknown error');
            // Even on error, clear local state and redirect
            if (typeof window !== 'undefined') {
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

              // Notify other tabs even on error — logout must propagate
              authChannel?.postMessage({ type: 'logout' });

              window.location.href = '/login';
            }
        }
    }

    async function register(userData: {
        user_id?: string;
        email: string;
        password: string;
        passwordConfirm: string;
        first_name: string;
        last_name: string;
        is_admin: boolean;
    }): Promise<void> {
        const { email, password, first_name, last_name } = userData;

        // Validate passwords match
        if (userData.password !== userData.passwordConfirm) {
            update(current => ({
                ...current,
                status: 'error',
                error_msg: 'Passwords do not match',
                isLoggedIn: false,
                isAdmin: false,
                isOwner: false,
            }));
            return;
        }

        try {
            // Backend handles Kratos registration flow
            // Send data in Kratos traits format
            const res = await fetch('/auth/email/signup', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({
                    traits: {
                        email,
                        name: { first: first_name, last: last_name }
                    },
                    password
                })
            });

            if (res.ok) {
                const data = await res.json() as SignupResponse;

                // Kratos may auto-login after registration (configured via hooks)
                if (data.session) {
                    const userInfo = mapKratosIdentityToUserInfo(data.session.identity);
                    update(current => ({
                        ...current,
                        user: userInfo,
                        isLoggedIn: true,
                        isAdmin: userInfo.admin ?? false,
                        isOwner: userInfo.is_owner ?? false,
                        status: 'success',
                        error_msg: '',
                    }));

                    // Redirect to dashboard or specified URL
                    if (typeof window !== 'undefined') {
                        window.location.href = data.redirect_url || '/dashboard';
                    }
                } else {
                    // Verification email sent, user needs to verify
                    const msg = `An email has been sent to ${email}. ` +
                        'Please check your email and click the link to verify your account. ' +
                        'Note: if you cannot find the email, check the Junk Mail section!';
                    alert(msg);
                    update(current => ({
                        ...current,
                        status: 'signup',
                        isLoggedIn: false,
                        error_msg: '',
                    }));
                }
            } else {
                const errorData = await res.json().catch(() => ({ message: 'Registration failed' }));
                const error_msg = errorData.message || 'Registration failed';
                alert(error_msg);
                update(current => ({
                    ...current,
                    error_msg: error_msg,
                    status: 'error',
                }));
            }
        } catch (error) {
            const error_msg = error instanceof Error ? error.message : 'Network error';
            alert('Network error: ' + error_msg);
            update(current => ({
                ...current,
                error_msg: error_msg,
                status: 'error',
            }));
        }
  }

  async function verifyEmail(token: string, _loc: string): Promise<VerifyEmailResults> {
    try {
        // Kratos verification is handled via its own flow
        // This endpoint checks verification status and may return session
        const response = await fetch(`/auth/verify-status?token=${token}`, {
            method: 'GET',
            credentials: 'include',
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({ message: 'Verification failed' }));
            console.error('Email verification failed:', errorData.message);
            return {
                status: false,
                error_msg: errorData.message || 'Email verification failed',
                redirect_url: '',
                LOC: 'SHD_ATH_VERIFY_FAIL'
            };
        }

        const data = await response.json() as VerifyStatusResponse;

        if (data.verified) {
            // Re-fetch session to get updated user state
            await checkAuthStatus();
            return {
                status: true,
                error_msg: '',
                redirect_url: data.redirect_url || '/dashboard',
                LOC: 'SHD_ATH_VERIFY_OK'
            };
        }

        // Verification not complete
        return {
            status: false,
            error_msg: 'Email verification pending',
            redirect_url: '',
            LOC: 'SHD_ATH_VERIFY_PENDING'
        };
    } catch (error) {
        const error_msg = error instanceof Error ? error.message : 'Unknown error';
        console.error('Email verification error:', error_msg);

        return {
            status: false,
            error_msg: error_msg,
            redirect_url: '',
            LOC: 'SHD_ATH_VERIFY_ERR'
        };
    }
  }

  // Impersonation persistence helpers (sessionStorage = tab-scoped, clears on tab close)
  const IMPERSONATION_KEY = 'mirai_impersonation';

  const persistImpersonation = (clientId: string, clientName: string): void => {
      try {
          sessionStorage.setItem(IMPERSONATION_KEY, JSON.stringify({ clientId, clientName }));
      } catch {
          // sessionStorage unavailable (SSR, private browsing quota, etc.) — silent fail
      }
  };

  const clearPersistedImpersonation = (): void => {
      try {
          sessionStorage.removeItem(IMPERSONATION_KEY);
      } catch {
          // silent fail
      }
  };

  const getPersistedImpersonation = (): { clientId: string; clientName: string } | null => {
      try {
          const raw = sessionStorage.getItem(IMPERSONATION_KEY);
          if (!raw) return null;
          const parsed = JSON.parse(raw);
          if (parsed && typeof parsed.clientId === 'string' && typeof parsed.clientName === 'string') {
              return parsed;
          }
          return null;
      } catch {
          return null;
      }
  };

  // Impersonation methods
  const startImpersonation = (clientId: string, clientName: string): void => {
      persistImpersonation(clientId, clientName);
      update(current => ({
          ...current,
          isImpersonating: true,
          impersonatedClientId: clientId,
          impersonatedClientName: clientName,
      }));
  };

  const stopImpersonation = (): void => {
      clearPersistedImpersonation();
      update(current => ({
          ...current,
          isImpersonating: false,
          impersonatedClientId: null,
          impersonatedClientName: null,
      }));
  };

  const getIsImpersonating = (): boolean => {
      return currentState.isImpersonating;
  };

  const getImpersonatedClientId = (): string | null => {
      return currentState.impersonatedClientId;
  };

  const getImpersonatedClientName = (): string | null => {
      return currentState.impersonatedClientName;
  };

  return {
      setUserInfo,
      login,
      logout,
      register,
      verifyEmail,
      checkAuthStatus,
      subscribe, // Expose the subscribe method
      getIsLoggedIn,
      getUser,
      getIsAdmin,
      getIsOwner,
      getBaseURL,
      ready: (): Promise<void> => readyPromise,
      startImpersonation,
      stopImpersonation,
      getIsImpersonating,
      getImpersonatedClientId,
      getImpersonatedClientName,
  };
}

// Export a singleton instance of the auth store
export const appAuthStore = createAuthStore();