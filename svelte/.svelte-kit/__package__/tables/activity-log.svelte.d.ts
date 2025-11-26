import type { UserInfo } from '../types/userinfo';
interface ActivityLogStore {
    isLoggedIn: boolean;
    user: UserInfo | null;
    isAdmin: boolean;
    setUserInfo: (userInfo: UserInfo | null) => void;
    register: (userData: {
        email: string;
        password: string;
        passwordConfirm: string;
        first_name: string;
        last_name: string;
    }) => Promise<void>;
}
export declare const authStore: ActivityLogStore;
export {};
