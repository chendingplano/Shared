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
import type { UserInfo } from '$lib/types/CommonTypes';
import type { JimoResponse } from '$lib/types/CommonTypes'

export let onNavigate = (path: string) => {}; // passed from the host app

interface AuthStoreState {
  isLoggedIn:       boolean;
  status:           string;
  error_msg:        string;
  user:             UserInfo | null;
  isAdmin:          boolean;
  isImpersonating:  boolean;
  impersonatedClientId: string | null;
}

export type LoginResults = {
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
    email:            string;
    password:         string;
    passwordConfirm:  string;
    first_name:       string;
    last_name:        string;
    is_admin:         boolean;
  }) => Promise<void>;
  // Add methods to check authentication state
  subscribe:        (run: (value: AuthStoreState) => void) => () => void;
  getIsLoggedIn:    () => boolean;
  getUser:          () => UserInfo | null;
  getIsAdmin:       () => boolean;
  ready:            () => Promise<void>;
  // Impersonation methods
  startImpersonation: (clientId: string) => void;
  stopImpersonation:  () => void;
  getIsImpersonating: () => boolean;
  getImpersonatedClientId: () => string | null;
}

function createAuthStore(): AuthStore {
    console.log("Create AuthStore (SHD_ATH_067)")

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
        isImpersonating: false,
        impersonatedClientId: null,
    });

    // Method to set user info
    const setUserInfo = (userInfo: UserInfo | null) => {
        update(currentState => ({
            ...currentState,
            user: userInfo,
            isLoggedIn: userInfo !== null && userInfo.user_status === 'login',
            isAdmin: userInfo?.admin?? false,
        }));
    };

    // Helper functions to get current state values
    let currentState: AuthStoreState = {
        isLoggedIn:     false,
        status:         'login',
        error_msg:      '',
        user:           null,
        isAdmin:        false,
        isImpersonating: false,
        impersonatedClientId: null,
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

    // ✅ NEW: Check if user is already logged in (via session/cookie)
    const checkAuthStatus = async () => {
        try {
            console.log("Fetch /auth/me (SHD_ATH_089)")
            const response = await fetch('/auth/me', {
                method: 'GET',
                credentials: 'include', // essential for cookies
            });

            console.log("Fetch /auth/me (SHD_ATH_131)")
            const resp_text = await response.text()
            if (response.ok) {
                console.log("Fetch /auth/me (SHD_ATH_134)")
                const resp_json = JSON.parse(resp_text) as JimoResponse
                const results_str = resp_json.results as string
                if (typeof results_str === 'string' && results_str.length > 0) {
                    const userInfo = JSON.parse(results_str) as UserInfo;

                    console.log("Fetch /auth/me success (SHD_ATH_102), user_name:" + userInfo.user_name +
                        "user name:" + userInfo.firstName+ " " + userInfo.lastName)
                    update(() => ({
                        isLoggedIn: !!userInfo,
                        status: 'success',
                        error_msg: '',
                        user: userInfo,
                        isAdmin: userInfo?.admin ?? false,
                        isImpersonating: false,
                        impersonatedClientId: null,
                    }));
                } else {
                    alert("User info is empty:" + resp_text)
                }
            } else {
                // Not logged in — leave state as is (logged out)
                console.log("Fetch /auth/me failed (SHD_ATH_116)")
                update(() => ({
                    isLoggedIn: false,
                    status: 'login',
                    user: null,
                    error_msg: 'failed check login',
                    isAdmin: false,
                    isImpersonating: false,
                    impersonatedClientId: null,
                }));
            }
        } catch (error) {
            console.warn('Auth check failed (SHD_ATH_120):', error);
            update(() => ({
                isLoggedIn: false,
                status: 'login',
                user: null,
                error_msg: 'check login, exception occurred',
                isAdmin: false,
                isImpersonating: false,
                impersonatedClientId: null,
            }));
        } finally {
            // 👇 Resolve the ready promise regardless of success/failure
            readyResolve();
        }
    };

    // DISABLED: Auto-check is causing infinite loop when /auth/me endpoint doesn't exist
    // This will be re-enabled when migrating from PocketBase to PostgreSQL
    // if (typeof window !== 'undefined') {
    //     checkAuthStatus();
    // } else {
    //     // SSR: resolve immediately (no auth check possible)
    //     readyResolve();
    // }

    // Temporary: Always resolve immediately without auth check
    readyResolve();

    async function login(email: string, password: string): Promise<LoginResults> {
        try {
            console.log("fetch login (SHD_ATH_193)")
            const response = await fetch('/auth/email/login', {
              method: 'POST',
              body: JSON.stringify({ email, password }),
              headers: { 'Content-Type': 'application/json' },
            });

            console.log("fetch login (SHD_ATH_200)")
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
            const user_name = user ? user.user_name:''
            const email = user ? user.email : ''
            console.log("user_name (SHD_AST_218):", user_name, "email:", email)
            alert("UserName1:" + user_name)
            alert("Email1:" + email)
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
              update(current => ({
                ...current,
                status: 'logout',
                user: null,
                isLoggedIn: false,
                isAdmin: false,
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
        email: string;
        password: string;
        passwordConfirm: string;
        first_name: string;
        last_name: string;
        is_admin: boolean;
    }): Promise<void> {
    try {
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
              user_name: email,
              password: password,
              firstName: first_name,
              lastName: last_name,
              email: email,
              user_type: "email",
              admin: is_admin,
              user_status: "signup"
            },
            isLoggedIn: false,
            isAdmin: false,
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

  // Impersonation methods
  const startImpersonation = (clientId: string): void => {
      update(current => ({
          ...current,
          isImpersonating: true,
          impersonatedClientId: clientId,
      }));
  };

  const stopImpersonation = (): void => {
      update(current => ({
          ...current,
          isImpersonating: false,
          impersonatedClientId: null,
      }));
  };

  const getIsImpersonating = (): boolean => {
      return currentState.isImpersonating;
  };

  const getImpersonatedClientId = (): string | null => {
      return currentState.impersonatedClientId;
  };

  return {
      setUserInfo,
      login,
      logout,
      register,
      subscribe, // Expose the subscribe method
      getIsLoggedIn,
      getUser,
      getIsAdmin,
      ready: (): Promise<void> => readyPromise,
      startImpersonation,
      stopImpersonation,
      getIsImpersonating,
      getImpersonatedClientId,
  };
}

// Export a singleton instance of the auth store
export const appAuthStore = createAuthStore();