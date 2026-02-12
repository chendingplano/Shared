# Implementation

## Backend - kratos.go

- After successful password login, checks if the identity has TOTP credentials configured
- If user has 2FA and session is AAL1, returns status: "2fa_required" with redirect to /verify-2fa
- Sets the session token cookie so the 2FA verification can use it

## Added HandleTOTPVerifyKratos

- Accepts TOTP code from user
- Creates an AAL2 login flow with Kratos using the existing session token
- Submits the TOTP code to upgrade the session from AAL1 to AAL2
- Returns success with redirect URL to dashboard

## Backend - router.go

Registered new route POST /auth/totp/verify for TOTP verification

## Frontend - login-01.svelte

- Updated handleEmailLogin to check for status: "2fa_required"
- Redirects to /verify-2fa when 2FA is needed

## Frontend - verify-2fa/+page.svelte (new file)

- Created a 2FA verification page with:
    - 6-digit code input
    - Form validation
    - API call to /auth/totp/verify
    - Error handling and redirect on success

## Backend - routes.go

Added /verify-2fa to public paths (accessible with AAL1 session)

## Flow
1. User enters email + password on login page
2. Backend validates credentials with Kratos (AAL1 session created)
3. Backend checks if user has TOTP configured
4. If yes: returns 2fa_required status, frontend redirects to /verify-2fa
5. User enters TOTP code from authenticator app
6. Backend creates AAL2 flow and verifies TOTP code
7. Session upgraded to AAL2, user redirected to /dashboard

# To Test
1. Rebuild and restart the ChenWeb server
2. Login with a user that has TOTP configured
3. You should be redirected to the 2FA page
4. Enter the TOTP code from your authenticator app

On success, you'll be redirected to the dashboard