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
import type { UserInfo, JimoResponse } from '../types/CommonTypes';

export let onNavigate = (path: string) => {}; // passed from the host app

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
    user_id:          string;
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

    // ✅ NEW: Check if user is already logged in (via session/cookie)
    const checkAuthStatus = async () => {
        // If already logged in with user info, skip the check
        if (currentState.isLoggedIn && currentState.user) {
            return;
        }

        try {
            const response = await fetch('/auth/me', {
                method: 'GET',
                credentials: 'include', // essential for cookies
            });

            const resp_text = await response.text()
            if (response.ok) {
                const resp_json = JSON.parse(resp_text) as JimoResponse
                const results_str = resp_json.results as string
                const base_url = resp_json.base_url
                if (base_url === null || base_url === "") {
                    console.error('Missing base_url in /auth/me response (SHD_ATH_185)')
                }
                if (typeof results_str === 'string' && results_str.length > 0) {
                    const userInfo = JSON.parse(results_str) as UserInfo;

                    // Restore impersonation from sessionStorage if present
                    const persisted = getPersistedImpersonation();
                    update(() => ({
                        isLoggedIn: !!userInfo,
                        status: 'success',
                        error_msg: '',
                        user: userInfo,
                        isAdmin: userInfo?.admin ?? false,
                        isOwner: userInfo?.is_owner ?? false,
                        baseURL: base_url ,
                        isImpersonating: persisted !== null,
                        impersonatedClientId: persisted?.clientId ?? null,
                        impersonatedClientName: persisted?.clientName ?? null,
                    }));
                }
            } else {
                // Not logged in — clear everything including persisted impersonation
                clearPersistedImpersonation();
                update(() => ({
                    isLoggedIn: false,
                    status: 'login',
                    user: null,
                    error_msg: 'failed check login',
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
            // 👇 Resolve the ready promise regardless of success/failure
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
            });
            if (!response.ok) {
                const errorData = await response.json();
                var error_msg: string
                if (errorData.message && typeof errorData.message === 'string') {
                  error_msg = errorData.message
                } else {
                  const status = response.status;
                  error_msg = response.statusText;
                }
                const result : LoginResults = {
                  status: false,
                  error_msg: error_msg,
                  redirect_url: "",
                  LOC: ""
                }
                return result
            }

            const userData = await response.json();
            const userInfo: UserInfo | null = userData.user || null;

            update((current) => {
              return {
                ...current,
                user: userInfo,
                isLoggedIn: userInfo !== null,
                isAdmin: userInfo?.admin ?? false,
                isOwner: userInfo?.is_owner?? false,
                status: 'success',
              }
            });

            let redirect_url = userData.redirect_url;
            alert("Login successful! redirect to:" + redirect_url);
            window.location.href = redirect_url;
            return {
              status : true,
              error_msg: "",
              redirect_url: redirect_url,
              LOC: userData.LOC || ""
            }
        } catch (error) {
            const error_msg = error instanceof Error ? error.message : "Unknown error"
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
              redirect_url: "",
              LOC: ""
            }
        }
    }

    async function logout(): Promise<void> {
        try {
            const user = currentState.user;
            const user_name = user ? user.name:''
            const email = user ? user.email : ''
            const response = await fetch('/auth/logout', {
              method: 'POST',
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ user_name, email}),
              credentials: 'include',
            });

            if (!response.ok) {
              throw new Error(`Server logout failed (Status: ${response.status})`);
            }

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
            }

            if (typeof window !== 'undefined') {
              window.location.href = '/login';
            }

        } catch (error) {
            console.error('Logout process failed:', error instanceof Error ? error.message : 'Unknown error');
        }
    }

    async function register(userData: {
        user_id: string;
        email: string;
        password: string;
        passwordConfirm: string;
        first_name: string;
        last_name: string;
        is_admin: boolean;
    }): Promise<void> {
    try {
        const user_id = userData.user_id
        const email = userData.email
        const password = userData.password
        const first_name = userData.first_name
        const last_name = userData.last_name
        const is_admin = userData.is_admin
        if (userData.password !== userData.passwordConfirm) {
          update(current => ({
            ...current,
            status: 'error',
            error_msg: 'Passwords do not match',
            user: {
              id: user_id,
              userName: email,
              password: password,
              firstName: first_name,
              lastName: last_name,
              email: email,
              authType: "email",
              admin: is_admin,
              userStatus: "signup"
            },
            isLoggedIn: false,
            isAdmin: false,
            isOwner: false,
          }));
        }

        const res = await fetch("/auth/email/signup", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ first_name, last_name, email, password })
        });

        if (res.ok) {
            const msg = "An email has been sent to your email:" + email +
              ". Please check your email and click the link to activate your account." +
              "Note: if you cannot find the email, check the Junk Mail section! (TAX_LFM_066)"
            alert(msg)
            update(current => ({
                ...current,
                status:"signup",
                isLoggedIn: false,
            }))
        } else {
            const error_msg = "Registration failed: " + await res.text();
            alert(error_msg);
            update(current => ({
                ...current,
                error_msg: error_msg,
                status: "error",
            }))
        }
    } catch (NetworkError) {
        alert('Network error: ' + (NetworkError instanceof Error ? NetworkError.message : 'unknown'));
    }
  }

  async function verifyEmail(token: string, _loc: string): Promise<VerifyEmailResults> {
    try {
        const response = await fetch(`/auth/email/verify?token=${token}&type=auth`, {
            method: 'GET',
            credentials: 'include', // Important: include cookies
        });

        const resp_text = await response.text();
        if (!response.ok) {
            console.error('Email verification failed:', resp_text);
            return {
                status: false,
                error_msg: resp_text || 'Email verification failed',
                redirect_url: '',
                LOC: 'SHD_ATH_395'
            };
        }

        // Parse the JSON response which contains user_info and redirect_url
        const resp_json = JSON.parse(resp_text);
        const base_url = resp_json.base_url
        if (base_url === null || base_url === '') {
            console.error('Missing base_url in email verify response (SHD_ATH_447)')
        }

        const redirect_url = resp_json.redirect_url || '/dashboard';
        const user_info_str = resp_json.user_info;

        if (typeof user_info_str === 'string' && user_info_str.length > 0) {
            const userInfo = JSON.parse(user_info_str) as UserInfo;

            update(() => ({
                isLoggedIn: true,
                status: 'success',
                error_msg: '',
                user: userInfo,
                isAdmin: userInfo?.admin ?? false,
                isOwner: userInfo?.is_owner?? false,
                baseURL: base_url,
                isImpersonating: false,
                impersonatedClientId: null,
                impersonatedClientName: null,
            }));

            return {
                status: true,
                error_msg: '',
                redirect_url: redirect_url,
                LOC: 'SHD_ATH_428'
            };
        }

        // user_info not present in response, verification succeeded but can't update store
        console.warn('Email verified but user_info missing in response (SHD_ATH_438)');
        return {
            status: false,
            error_msg: 'Email verified but failed to get user info',
            redirect_url: '',
            LOC: 'SHD_ATH_438'
        };
    } catch (error) {
        const error_msg = error instanceof Error ? error.message : "Unknown error";
        console.error('Email verification error:', error_msg);

        return {
            status: false,
            error_msg: error_msg,
            redirect_url: '',
            LOC: 'SHD_ATH_448'
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