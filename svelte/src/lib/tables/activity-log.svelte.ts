import type { UserInfo } from '../types/userinfo';

/*
How to use the register function:
  async function handleRegister() {
    if (password !== passwordConfirm) {
      errorMessage = 'Passwords do not match';
      return;
    }

    isSubmitting = true;
    errorMessage = '';

    try {
      await authStore.register({
        email,
        password,
        passwordConfirm,
        first_name,
        last_name
      });
      
      // Registration successful!
      // The authStore will automatically update isLoggedIn and user
      console.log('Registration successful');
    } catch (error) {
      // Handle registration errors
      errorMessage = error instanceof Error ? error.message : 'Registration failed';
    } finally {
      isSubmitting = false;
    }
  }
*/

interface ActivityLogStore {
  isLoggedIn:     boolean;
  user:           UserInfo | null;
  isAdmin:        boolean;
  setUserInfo:    (userInfo: UserInfo | null) => void;
  register: (userData: {
    email:            string;
    password:         string;
    passwordConfirm:  string;
    first_name:       string;
    last_name:        string;
  }) => Promise<void>;
}

function createActivityLogStore(): ActivityLogStore {
  // Initialize state from current PocketBase auth store
  let isLoggedIn = $state<boolean>(false);
  let user = $state<UserInfo | null>(null);
  let error_msg = $state<string | null>(null);
  let status = $state<'login' | 'register' | 'forgot' | 'loggedin' | 'error' | 'pending'>('login');

  // Subscribe to PocketBase auth changes once, globally
  function setUserInfo(userInfo: UserInfo | null) {
    user = userInfo;
    if (user === null) {
      isLoggedIn = false;
    }
    else {
      isLoggedIn = true;
    }
  }

  async function addRecord(email: string, password: string): Promise<void> {
    // Authenticate with email and password
    // State will automatically update via the onChange subscription
  }

  function logout(): void {
    user = null;
  }

  async function register(userData: {
    email: string;
    password: string;
    passwordConfirm: string;
    first_name: string;
    last_name: string;
  }): Promise<void> {
    try {
        if (userData.password !== userData.passwordConfirm) {
          error_msg = 'Passwords do not match';
          status = 'error';
          return;
        }

        const first_name = userData.first_name;
        const last_name = userData.last_name;
        const email = userData.email;
        const password = userData.password;
        const res = await fetch("http://localhost:8080/auth/email/signup", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ first_name, last_name, email, password })
        });

        if (res.ok) {
          const msg = "An email has been sent to your email:" + email +
            ". Please check your email and click the link to activate your account." +
            "Note: if you cannot find the email, check the Junk Mail section! (TAX_LFM_066)"
          alert(msg)
          status = "pending";
        } else {
          error_msg = "Registration failed: " + await res.text();
          status = "error";
          alert(error_msg);
        }
      } catch (NetworkError) {
        alert('Network error: ' + (NetworkError instanceof Error ? NetworkError.message : 'unknown'));
      }
  }

  return {
    get isLoggedIn() {
      return isLoggedIn;
    },
    get user() {
      return user;
    },
    get isAdmin() {
      if (!user) return false;
      return user.user_name === 'admin';
    },
    setUserInfo,
    register,
  };
}

// Export a singleton instance of the auth store
export const authStore = createActivityLogStore();
