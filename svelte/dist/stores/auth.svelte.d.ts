import type { UserInfo } from '@chendingplano/shared';
export declare let onNavigate: (path: string) => void;
interface AuthStoreState {
    isLoggedIn: boolean;
    status: string;
    error_msg: string;
    user: UserInfo | null;
    isAdmin: boolean;
}
export type LoginResults = {
    status: boolean;
    error_msg: string;
    redirect_url: string;
};
interface AuthStore {
    setUserInfo: (userInfo: UserInfo | null) => void;
    login: (email: string, password: string) => Promise<LoginResults>;
    logout: () => void;
    register: (userData: {
        email: string;
        password: string;
        passwordConfirm: string;
        first_name: string;
        last_name: string;
    }) => Promise<void>;
    subscribe: (run: (value: AuthStoreState) => void) => () => void;
    getIsLoggedIn: () => boolean;
    getUser: () => UserInfo | null;
    getIsAdmin: () => boolean;
}
export declare const appAuthStore: AuthStore;
export {};
