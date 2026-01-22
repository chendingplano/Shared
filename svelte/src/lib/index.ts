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
export { appAuthStore } from './stores/auth.svelte';
export { db_store } from './stores/dbstore';
export type {LoginResults} from './stores/auth.svelte'
export {GetStoreByName, StoreMap} from './stores/InMemStores'
export type {RecordInfo, InMemStoreDef} from './stores/InMemStores'
export { 
    type EmailSignupResponse, 
    type JimoRequest,
    type ResourceDef,
    type JimoResponse,
    type OrderbyDef,
    type CondDef,
    type JoinDef,
    type UpdateDef,
    type UserInfo,
    type FieldDef,
    CustomHttpStatus,
    type UpdateWithCondDef } from './types/CommonTypes';
export { 
    SafeJsonParseAsObject, 
    ParseObjectOrArray,
    GetAllKeys,
    IsValidNonEmptyString,
    ParseConfigFile
} from './utils/UtilFuncs';
export { cond_builder, parseCondition } from './stores/cond_builder'
export { join_builder } from './stores/join_builder'
export { update_builder } from './stores/update_builder'
export { orderby_builder } from './stores/orderby_builder'
export { query_builder } from './stores/query_builder'

export { TableUsersFieldDefs } from './db-scripts/table-users'