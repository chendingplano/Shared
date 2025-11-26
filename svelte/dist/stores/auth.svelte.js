import { writable } from 'svelte/store';
export let onNavigate = (path) => { }; // passed from the host app
function createAuthStore() {
    // Initialize the writable store with the state object
    const { subscribe, set, update } = writable({
        isLoggedIn: false,
        status: 'login',
        error_msg: '',
        user: null,
        isAdmin: false,
    });
    // Method to set user info
    const setUserInfo = (userInfo) => {
        update(currentState => ({
            ...currentState,
            user: userInfo,
            isLoggedIn: userInfo !== null && userInfo.user_status === 'login',
            isAdmin: userInfo?.user_role === 'admin' || false,
        }));
    };
    // Helper functions to get current state values
    let currentState = {
        isLoggedIn: false,
        status: 'login',
        error_msg: '',
        user: null,
        isAdmin: false,
    };
    // Subscribe to state changes to keep currentState updated
    const unsubscribe = subscribe(state => {
        currentState = state;
    });
    const getIsLoggedIn = () => {
        return currentState.isLoggedIn;
    };
    const getUser = () => {
        return currentState.user;
    };
    const getIsAdmin = () => {
        return currentState.isAdmin;
    };
    async function login(email, password) {
        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                body: JSON.stringify({ email, password }),
                headers: { 'Content-Type': 'application/json' },
            });
            if (!response.ok) {
                const errorData = await response.json();
                var error_msg;
                if (errorData.message && typeof errorData.message === 'string') {
                    error_msg = errorData.message;
                }
                else {
                    const status = response.status;
                    error_msg = response.statusText;
                }
                const result = {
                    status: false,
                    error_msg: error_msg,
                    redirect_url: ""
                };
                return result;
            }
            const userData = await response.json();
            const userInfo = userData.user || null;
            update((current) => {
                return {
                    ...current,
                    user: userInfo,
                    isLoggedIn: userInfo !== null,
                    isAdmin: userInfo?.user_role === 'admin' || false,
                    status: 'success',
                };
            });
            let redirect_url = userData.redirect_url;
            alert("Login successful! redirect to:" + redirect_url);
            window.location.href = redirect_url;
            return {
                status: true,
                error_msg: "",
                redirect_url: redirect_url
            };
        }
        catch (error) {
            const error_msg = error instanceof Error ? error.message : "Unknown error";
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
                redirect_url: ""
            };
        }
    }
    async function logout() {
        try {
            const user = currentState.user;
            const user_name = user ? user.user_name : '';
            const email = user ? user.email : '';
            console.log("user_name:", user_name, "email:", email);
            alert("UserName1:" + user_name);
            alert("Email1:" + email);
            const response = await fetch('/auth/logout', {
                method: 'POST',
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ user_name, email }),
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
                window.location.href = 'http://localhost:5173/login';
            }
        }
        catch (error) {
            console.error('Logout process failed:', error instanceof Error ? error.message : 'Unknown error');
        }
    }
    async function register(userData) {
        try {
            const email = userData.email;
            const password = userData.password;
            const first_name = userData.first_name;
            const last_name = userData.last_name;
            if (userData.password !== userData.passwordConfirm) {
                update(current => ({
                    ...current,
                    status: 'error',
                    error_msg: 'Passwords do not match',
                    user: {
                        user_name: email,
                        password: password,
                        first_name: first_name,
                        last_name: last_name,
                        email: email,
                        user_type: "email",
                        user_role: '',
                        redirect_url: '',
                        user_status: "signup"
                    },
                    isLoggedIn: false,
                    isAdmin: false,
                }));
            }
            const res = await fetch("http://localhost:8080/auth/email/signup", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ first_name, last_name, email, password })
            });
            if (res.ok) {
                const msg = "An email has been sent to your email:" + email +
                    ". Please check your email and click the link to activate your account." +
                    "Note: if you cannot find the email, check the Junk Mail section! (TAX_LFM_066)";
                alert(msg);
                update(current => ({
                    ...current,
                    status: "signup",
                    isLoggedIn: false,
                }));
            }
            else {
                const error_msg = "Registration failed: " + await res.text();
                alert(error_msg);
                update(current => ({
                    ...current,
                    error_msg: error_msg,
                    status: "error",
                }));
            }
        }
        catch (NetworkError) {
            alert('Network error: ' + (NetworkError instanceof Error ? NetworkError.message : 'unknown'));
        }
    }
    return {
        setUserInfo,
        login,
        logout,
        register,
        subscribe, // Expose the subscribe method
        getIsLoggedIn,
        getUser,
        getIsAdmin,
    };
}
// Export a singleton instance of the auth store
export const appAuthStore = createAuthStore();
