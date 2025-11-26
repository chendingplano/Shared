// src/lib/utils/auth.ts
// The purpose of this utility is to check if a user is authenticated
// by making a request to the backend API endpoint /api/v1/auth/me.
// It caches the result to avoid redundant network requests.
// 
// Cache Management:
// - setIsAuthenticated(value: boolean): Call this function to update
//   the cached authentication status when the auth state changes
//   (e.g., after login or logout).
// - clearAuthCache(): Call this function to reset the cached status
//   (e.g., on logout or when the session might have changed).
let _isAuthenticated = null;
let _emailVerifyUrl = "";
// Call this function when auth state changes (e.g., after login/logout)
export function setIsAuthenticated(value) {
    _isAuthenticated = value;
}
export function setEmailVerifyUrl(value) {
    _emailVerifyUrl = value;
}
export function getEmailVerifyUrl() {
    return _emailVerifyUrl;
}
// Reset cache (e.g., on logout or when session might have changed)
export function clearAuthCache() {
    _isAuthenticated = null;
}
// Check auth status (uses cache if available, otherwise fetches)
export async function isAuthenticated() {
    if (_isAuthenticated !== null) {
        return _isAuthenticated;
    }
    try {
        const res = await fetch('/auth/me', {
            method: 'GET',
            credentials: 'include'
        });
        _isAuthenticated = res.ok;
        return _isAuthenticated;
    }
    catch (error) {
        _isAuthenticated = false;
        return false;
    }
}
