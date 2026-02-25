# Migrate User Management from users Table to Kratos Identities
## Context
Authentication (login, signup, forgot password) has already been migrated to Kratos. However, the user management routes still read/write to the old users table in the tax database. New users registered via Kratos won't appear in the admin user management pages because those pages query the old table. This migration makes all user management handlers use the Kratos Admin API instead.

## Key Insight:
The API response format (UserInfo) stays the same. Backend handlers transform Kratos identity data into UserInfo, so frontend changes are minimal.

## Field Mapping: users Table → Kratos Identity
| UserInfo Field	| Old Source (users table)	| New Source (Kratos) |
|-------------------|---------------------------|---------------------|
| id	| users.id	| identity.id |
| email	| users.email	| identity.traits.email |
| first_name	| users.first_name	| identity.traits.name.first |
| last_name	| users.last_name	| identity.traits.name.last |
| admin	| users.admin	| identity.metadata_public.admin (already used) |
| is_owner	| users.is_owner	| identity.metadata_public.is_owner (already used) |
| avatar	| users.avatar	| identity.metadata_public.avatar (new) |
| user_status	| users.user_status	| identity.state ("active"/"inactive") |
------------

Note: most customized data fields are in identity.metadata_public, which is a JSON document. More can be added on as needed.

## Step 1: Add Kratos Admin API Helpers
**File: shared/go/api/auth/kratos.go**

Add these functions following the existing pattern of direct HTTP calls to KRATOS_ADMIN_URL (same pattern as HandleResetPasswordConfirmKratos at line 1964):

```go
KratosIdentityToUserInfo(identity map[string]interface{}) *ApiTypes.UserInfo
```

- Shared conversion: extracts traits, metadata_public, state → UserInfo
- Reuse this across all handlers to avoid duplicating mapping logic

```go
KratosListAllIdentities(logger) ([]*ApiTypes.UserInfo, error)

GET {KRATOS_ADMIN_URL}/admin/identities?per_page=1000
```
Returns all identities as []*UserInfo via KratosIdentityToUserInfo

```go
KratosGetIdentityByID(logger, id) (*ApiTypes.UserInfo, error)

GET {KRATOS_ADMIN_URL}/admin/identities/{id}
```

Returns single identity as *UserInfo

```go
KratosUpdateIdentity(logger, id, updates) error
```

- GET-then-PUT pattern (fetch current identity, merge updates, PUT back)
- updates param is a struct with optional fields: Traits, MetadataPublic, State
- Preserves all unmodified fields (schema_id, metadata_admin, credentials)

```go
KratosDeleteIdentitySessions(logger, id) error
DELETE {KRATOS_ADMIN_URL}/admin/identities/{id}/sessions
```

Replaces sysdatastores.RefreshTokenKey() for session invalidation

## Step 2: Update Auth Session Responses to Include avatar and state
### 2a. HandleAuthMeKratos (kratos.go:651)
Add avatar extraction from identity.MetadataPublic and include in response at line 723:

```go
"metadata_public": map[string]interface{}{
    "admin":    isAdmin,
    "is_owner": isOwner,
    "avatar":   avatar,    // NEW
},
```
"state": identity.State,   // NEW

### 2b. HandleEmailLoginKratosBase (kratos.go:622)

Same change — add avatar to metadata_public block and state to identity in the login response.

### 2c. IsAuthenticatedKratosFromRC (kratos.go:1433)

Extract avatar from MetadataPublic, map identity.State to UserStatus:

Avatar:     avatar,      // was empty
UserStatus: userStatus,  // was hardcoded "active"

### 2d. IsAuthenticatedKratos (kratos.go:1337)

Same changes as 2c. Also fix pre-existing bug: set IsOwner in UserInfo (currently only logged, not set).

## Step 3: Update Frontend Auth Store

**File: shared/svelte/src/lib/stores/auth.svelte.ts**

### 3a. Update KratosIdentity interface (line 24)
Add avatar to metadata_public and add state field:

```go
metadata_public?: {
    admin?: boolean;
    is_owner?: boolean;
    avatar?: string;     // NEW
};
```

state?: string;          // NEW

### 3b. Update mapKratosIdentityToUserInfo (line 137)

user_status: identity.state ?? 'active',  // was hardcoded 'active'
avatar: metadata.avatar,                   // NEW

## Step 4: Migrate Backend Handlers
### 4a. HandleListUsers (user_management_handlers.go:14)
Replace sysdatastores.GetAllUsers(rc) → auth.KratosListAllIdentities(logger)

### 4b. HandleGetUser (users_handler.go:113)
Replace sysdatastores.GetUserInfoByUserID(rc, userID) → auth.KratosGetIdentityByID(logger, userID)

### 4c. HandleGetAdminUsers (users_handler.go:15)
Replace sysdatastores.GetAllAdmins(rc, isAdmin):

- Call auth.KratosListAllIdentities(logger)
- Filter by user.Admin == isAdmin

Consultant filtering logic (appdatastores.GetAllConsultantUserIDs) unchanged

### 4d. HandleToggleAdmin (user_management_handlers.go:49)
Replace GetUserInfoByUserID + UpsertUser:

auth.KratosGetIdentityByID(logger, userId) — fetch current state for audit log
auth.KratosUpdateIdentity(logger, userId, {MetadataPublic: {"admin": body.Admin}}) — merge update
Audit log (appdatastores.LogUserAction) unchanged
4e. HandleToggleOwner (user_management_handlers.go:130)
Replace GetUserInfoByUserID + CountOwners + UpsertUser:

auth.KratosGetIdentityByID(logger, userId) — fetch for validation
Count owners: auth.KratosListAllIdentities(logger) then count where IsOwner == true
auth.KratosUpdateIdentity(logger, userId, {MetadataPublic: {"is_owner": body.IsOwner}})
Self-modification check and audit log unchanged
4f. HandleToggleStatus (user_management_handlers.go:231)
Replace GetUserInfoByUserID + UpsertUser + RefreshTokenKey:

auth.KratosGetIdentityByID(logger, userId) — fetch current state
auth.KratosUpdateIdentity(logger, userId, {State: body.Status}) — set active/inactive
If deactivating: auth.KratosDeleteIdentitySessions(logger, userId) — force logout
Audit log unchanged
4g. HandleUpdateUserProfile (user_profile_handler.go:63)
Replace rc.GetUserInfoByUserID + sysdatastores.UpdateUserProfile:

auth.KratosGetIdentityByID(logger, userID) — get existing avatar for file cleanup
Avatar file upload/deletion logic — unchanged (stays on disk)
Single auth.KratosUpdateIdentity(logger, userID, {Traits: updatedTraits, MetadataPublic: {"avatar": filename}}) — updates name + avatar in one call
Return full UserInfo from updated identity (not the limited UserProfileResponse)
Step 5: Add Import for auth Package in Handler Files
Files:

tax/server/api/handlers/user_management_handlers.go — add "github.com/chendingplano/shared/go/api/auth"
tax/server/api/handlers/users_handler.go — add same import
tax/server/api/handlers/user_profile_handler.go — add same import
Remove unused sysdatastores imports from these files after migration.

## Files Modified
| File	| Changes |
|-------|---------|
| shared/go/api/auth/kratos.go	| Add 5 Kratos Admin API helpers + update 4 auth functions |
| shared/svelte/src/lib/stores/auth.svelte.ts	| Update interface + mapping function |
| tax/server/api/handlers/user_management_handlers.go	| Migrate 4 handlers |
| tax/server/api/handlers/users_handler.go	| Migrate 2 handlers |
| tax/server/api/handlers/user_profile_handler.go	| Migrate 1 handler |
| Files NOT Modified (no frontend changes needed) |
| tax/web/src/lib/api/users.ts | API response format unchanged |
| tax/web/src/routes/(admin)/admin/users/+page.svelte | consumes same UserInfo |
| tax/web/src/routes/(admin)/admin/users/+page.ts | fetches same endpoint |
-------

## Migrate echo_factory.go 
### New Kratos Helper Functions Added to kratos.go:
1. KratosGetIdentityByEmail (lines ~2501-2548) - Fetches a Kratos identity by email using credentials_identifier query parameter
2. KratosMarkUserVerified (lines ~2717-2738) - Marks a user as verified by setting their identity state to "active"
3. KratosUpdateIdentityWrapper (lines ~2740-2757) - Wrapper function to match EchoFactory function pointer signature

### Function Pointers Added to echo_factory.go:
Added function pointer definitions (lines ~48-65) to break import cycles:

- GetUserInfoByEmailFunc - Get user by email
- KratosMarkUserVerifiedFunc - Mark user verified
- KratosUpdateIdentityFunc - Update identity traits/metadata/state

### Migrated Functions in echo_factory.go:
1. GetUserInfoByEmail (lines ~359-398) ✅

    - Uses KratosGetIdentityByEmail when AUTH_USE_KRATOS=true
    - Falls back to old implementation otherwise

2. GetUserInfoByToken (lines ~335-358) ✅

    - Logs warning that VTokens are managed by Kratos flows
    - Falls through to old implementation for backwards compatibility (portal invites)

3. UpdatePassword (lines ~194-268) ✅

    - Returns error when AUTH_USE_KRATOS=true, directing users to use recovery flows
    - Password updates in Kratos must go through HandleResetPasswordConfirmKratos

4. VerifyUserPassword (lines ~270-333) ✅

    - Returns error when AUTH_USE_KRATOS=true, directing users to use login flows
    - Password verification in Kratos goes through HandleEmailLoginKratos

5. MarkUserVerified (lines ~449-462) ✅

    - Uses KratosMarkUserVerifiedFunc to update identity state to "active"
    - Used for admin override or manual verification

6. UpdateTokenByEmail (lines ~464-477) ✅

There are two types of tokens: authentication tokens and application tokens. Authentication tokens are managed by Kratos. If AUTH_USE_KRATOS == true, it will report errors.

    - Returns error when AUTH_USE_KRATOS=true
    - Verification tokens are managed by Kratos flows, not our database

Add a new function UpdateAppTokenByEmail(...). This is used for app tokens, such as invite users (used in Mirai).

7. UpsertUser (lines ~479-601 for Kratos, continues with old implementation) ✅

    - Comprehensive Kratos implementation that:
        - Updates traits (email, firstName, lastName)
        - Updates metadata_public (admin, is_owner, avatar)
        - Updates state (active/inactive)
    - User creation should go through signup flows
    - Password setting returns error (must use flows)
    - Falls back to old implementation for non-Kratos mode

8. SaveSession (lines ~434-457) ✅

    - Returns early when AUTH_USE_KRATOS=true with info log
    - Sessions are managed by Kratos, not our database
    - Suggests using activity logs for audit tracking if needed

### Registration in kratos.go InitKratosClient:
All new function pointers are registered (lines ~53-56):

    - EchoFactory.GetUserInfoByUserIDFunc = KratosGetIdentityByID
    - EchoFactory.GetUserInfoByEmailFunc = KratosGetIdentityByEmail
    - EchoFactory.KratosMarkUserVerifiedFunc = KratosMarkUserVerified
    - EchoFactory.KratosUpdateIdentityFunc = KratosUpdateIdentityWrapper

## Key Design Decisions

1. Import Cycle Avoidance: Used function pointers registered at runtime to avoid circular dependencies between EchoFactory and auth packages
2. Backwards Compatibility: All functions check AUTH_USE_KRATOS environment variable and fall back to old implementations when not using Kratos
3 .Flow-Based Operations: Password and verification token operations return errors directing users to use Kratos flows instead of direct API calls
4. Session Management: With Kratos, sessions are managed by Kratos itself, so we skip saving to our database
5. Metadata Storage: Custom fields (admin, is_owner, avatar) are stored in Kratos metadata_public

All migrations follow the same pattern as the example GetUserInfoByUserID function you provided. The system will seamlessly use Kratos when AUTH_USE_KRATOS=true and fall back to the old authentication system otherwise.

All other frontend files consuming user APIs

## Dropping the users table
Keep the table but erase all records. The table can be used as references for migration.

Data migration of existing users table rows into Kratos identities