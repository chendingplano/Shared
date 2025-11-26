// ~/Workspace/Shared/src/lib/index.ts
// Reexport your entry components here
// Re-export anything you want included in the package
export { isAuthenticated, 
    setIsAuthenticated, 
    clearAuthCache,
    setEmailVerifyUrl,
    getEmailVerifyUrl
} from './utils/auth';

export { default as EmailVerifyPage } from './components/EmailVerifyPage.svelte';
export type { UserInfo } from './types/userinfo';
export { appAuthStore } from './stores/auth.svelte';
export { db_store } from './stores/dbstore';
export type {LoginResults} from './stores/auth.svelte'
export {GetStoreByName, StoreMap} from './stores/InMemStores'
export type {RecordInfo, InMemStoreDef} from './stores/InMemStores'
export type { 
    EmailSignupResponse, 
    JimoRequest,
    JimoResponse } from './types/CommonTypes';
export { 
    SafeJsonParseAsObject, 
    ParseObjectOrArray,
    GetAllKeys } from './utils/UtilFuncs';