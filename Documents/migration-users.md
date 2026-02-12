Migrate User Management from users Table to Kratos Identities
Context
Authentication (login, signup, forgot password) has already been migrated to Kratos. However, the user management routes still read/write to the old users table in the tax database. New users registered via Kratos won't appear in the admin user management pages because those pages query the old table. This migration makes all user management handlers use the Kratos Admin API instead.

Key insight: The API response format (UserInfo) stays the same. Backend handlers transform Kratos identity data into UserInfo, so frontend changes are minimal.

Field Mapping: users Table → Kratos Identity
UserInfo Field	Old Source (users table)	New Source (Kratos)
id	users.id	identity.id
email	users.email	identity.traits.email
first_name	users.first_name	identity.traits.name.first
last_name	users.last_name	identity.traits.name.last
admin	users.admin	identity.metadata_public.admin (already used)
is_owner	users.is_owner	identity.metadata_public.is_owner (already used)
avatar	users.avatar	identity.metadata_public.avatar (new)
user_status	users.user_status	identity.state ("active"/"inactive")
Step 1: Add Kratos Admin API Helpers
File: shared/go/api/auth/kratos.go (append to existing file)

Add these functions following the existing pattern of direct HTTP calls to KRATOS_ADMIN_URL (same pattern as HandleResetPasswordConfirmKratos at line 1964):

KratosIdentityToUserInfo(identity map[string]interface{}) *ApiTypes.UserInfo

Shared conversion: extracts traits, metadata_public, state → UserInfo
Reuse this across all handlers to avoid duplicating mapping logic
KratosListAllIdentities(logger) ([]*ApiTypes.UserInfo, error)

GET {KRATOS_ADMIN_URL}/admin/identities?per_page=1000
Returns all identities as []*UserInfo via KratosIdentityToUserInfo
KratosGetIdentityByID(logger, id) (*ApiTypes.UserInfo, error)

GET {KRATOS_ADMIN_URL}/admin/identities/{id}
Returns single identity as *UserInfo
KratosUpdateIdentity(logger, id, updates) error

GET-then-PUT pattern (fetch current identity, merge updates, PUT back)
updates param is a struct with optional fields: Traits, MetadataPublic, State
Preserves all unmodified fields (schema_id, metadata_admin, credentials)
KratosDeleteIdentitySessions(logger, id) error

DELETE {KRATOS_ADMIN_URL}/admin/identities/{id}/sessions
Replaces sysdatastores.RefreshTokenKey() for session invalidation
Step 2: Update Auth Session Responses to Include avatar and state
2a. HandleAuthMeKratos (kratos.go:651)
Add avatar extraction from identity.MetadataPublic and include in response at line 723:


"metadata_public": map[string]interface{}{
    "admin":    isAdmin,
    "is_owner": isOwner,
    "avatar":   avatar,    // NEW
},
"state": identity.State,   // NEW
2b. HandleEmailLoginKratosBase (kratos.go:622)
Same change — add avatar to metadata_public block and state to identity in the login response.

2c. IsAuthenticatedKratosFromRC (kratos.go:1433)
Extract avatar from MetadataPublic, map identity.State to UserStatus:


Avatar:     avatar,      // was empty
UserStatus: userStatus,  // was hardcoded "active"
2d. IsAuthenticatedKratos (kratos.go:1337)
Same changes as 2c. Also fix pre-existing bug: set IsOwner in UserInfo (currently only logged, not set).

Step 3: Update Frontend Auth Store
File: shared/svelte/src/lib/stores/auth.svelte.ts

3a. Update KratosIdentity interface (line 24)
Add avatar to metadata_public and add state field:


metadata_public?: {
    admin?: boolean;
    is_owner?: boolean;
    avatar?: string;     // NEW
};
state?: string;          // NEW
3b. Update mapKratosIdentityToUserInfo (line 137)

user_status: identity.state ?? 'active',  // was hardcoded 'active'
avatar: metadata.avatar,                   // NEW
Step 4: Migrate Backend Handlers
4a. HandleListUsers (user_management_handlers.go:14)
Replace sysdatastores.GetAllUsers(rc) → auth.KratosListAllIdentities(logger)

4b. HandleGetUser (users_handler.go:113)
Replace sysdatastores.GetUserInfoByUserID(rc, userID) → auth.KratosGetIdentityByID(logger, userID)

4c. HandleGetAdminUsers (users_handler.go:15)
Replace sysdatastores.GetAllAdmins(rc, isAdmin):

Call auth.KratosListAllIdentities(logger)
Filter by user.Admin == isAdmin
Consultant filtering logic (appdatastores.GetAllConsultantUserIDs) unchanged
4d. HandleToggleAdmin (user_management_handlers.go:49)
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

Files Modified
File	Changes
shared/go/api/auth/kratos.go	Add 5 Kratos Admin API helpers + update 4 auth functions
shared/svelte/src/lib/stores/auth.svelte.ts	Update interface + mapping function
tax/server/api/handlers/user_management_handlers.go	Migrate 4 handlers
tax/server/api/handlers/users_handler.go	Migrate 2 handlers
tax/server/api/handlers/user_profile_handler.go	Migrate 1 handler
Files NOT Modified (no frontend changes needed)
tax/web/src/lib/api/users.ts — API response format unchanged
tax/web/src/routes/(admin)/admin/users/+page.svelte — consumes same UserInfo
tax/web/src/routes/(admin)/admin/users/+page.ts — fetches same endpoint
All other frontend files consuming user APIs
Out of Scope (follow-up)
rc.GetUserInfoByUserID() calls in client_membership_handlers.go, booking_handler.go, availability_handler.go, calendar_block_handler.go — these still use the users table and should be migrated separately
Dropping the users table — keep it as fallback until all references are migrated
Data migration of existing users table rows into Kratos identities