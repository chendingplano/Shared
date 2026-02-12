# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 1. Overview

The `shared/` directory contains **two shared libraries** used across all applications in the workspace:

1. **Go Library** (`go/`) - Backend utilities for authentication, database access, and request handling
2. **Svelte Library** (`svelte/`) - Frontend components, stores, and TypeScript utilities

These libraries provide common functionality to avoid code duplication across projects like `tax`, `ChenWeb`, and `deepdoc`.

## 2. Directory Structure

```
shared/
├── go/                      # Go shared library
│   ├── api/                 # Core API packages
│   │   ├── ApiTypes/        # Type definitions and database configs
│   │   ├── auth/            # Authentication (Google, GitHub, email, tokens)
│   │   ├── databaseutil/    # Database utilities and table management
│   │   ├── RequestHandlers/ # HTTP request handlers (PG/MySQL)
│   │   ├── security/        # Authorization
│   │   ├── stores/          # Resource and user account stores
│   │   ├── sysdatastores/   # System data stores
│   │   ├── loggerutil/      # Logging utilities
│   │   ├── EchoFactory/     # Echo context factory
│   │   └── libmanager/      # Library configuration manager
│   └── authmiddleware/      # Echo authentication middleware
├── svelte/                  # Svelte 5 shared library
│   └── src/lib/
│       ├── components/      # Reusable Svelte components
│       ├── stores/          # Svelte stores and query builders
│       ├── types/           # TypeScript type definitions
│       ├── utils/           # Utility functions
│       └── db-scripts/      # Database schema definitions
├── libconfig.toml           # Shared library configuration
└── mise.toml                # Task runner configuration
```

## 3. Go Library (`shared/go`)

### Module Information

**Module:** `github.com/chendingplano/shared/go`
**Go Version:** 1.25.0

### Core Packages

#### `api/ApiTypes`

Central type definitions and global state for the shared library.

**Key Types:**
- `RequestContext` - Framework-agnostic request context interface
- `UserInfo` - User account information (synced with TypeScript)
- `JimoRequest`, `JimoResponse` - Generic request/response types
- `QueryRequest`, `InsertRequest`, `UpdateRequest`, `DeleteRequest` - Database operation types
- `CondDef`, `OrderbyDef`, `JoinDef`, `UpdateDef` - SQL query builder types
- `FieldDef` - Field definition (synced with TypeScript)
- `DBConfig`, `DatabaseInfoDef` - Database configuration

**Global Variables:**
```go
var PG_DB_miner *sql.DB      // PostgreSQL connection pool
var MySql_DB_miner *sql.DB   // MySQL connection pool
var DatabaseInfo DatabaseInfoDef
var LibConfig LibConfigDef
```

**Usage:**
```go
import "github.com/chendingplano/shared/go/api/ApiTypes"

// Access database pool
db := ApiTypes.PG_DB_miner

// Get configured table names
usersTable := ApiTypes.GetUsersTableName()
sessionsTable := ApiTypes.GetSessionsTableName()
```

#### `api/auth`

Authentication utilities supporting multiple providers.

**Features:**
- Google OAuth (`google.go`)
- GitHub OAuth (`github.go`)
- Email-based authentication (`email.go`)
- JWT token management (`tokens.go`)
- Authentication utilities (`auth-util.go`, `authme.go`, `authinfo.go`)

**Usage:**
```go
import "github.com/chendingplano/shared/go/api/auth"

// Generate auth token
token, err := auth.GenerateToken(userEmail)

// Validate token
userInfo, valid := auth.ValidateToken(token)
```

#### `authmiddleware`

Echo middleware for protecting routes with authentication.

**Key Functions:**
- `AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc` - Route protection middleware
- `IsAuthenticated(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error)` - Check authentication status
- `IsHTMLRequest(c echo.Context) bool` - Detect HTML vs API requests

**Usage:**
```go
import (
    "github.com/chendingplano/shared/go/authmiddleware"
    "github.com/labstack/echo/v4"
)

func main() {
    // Initialize authenticator
    authmiddleware.Init()

    e := echo.New()

    // Protected routes
    protected := e.Group("/api")
    protected.Use(authmiddleware.AuthMiddleware)

    // Routes here require authentication
    protected.GET("/profile", getProfile)
}
```

**How it works:**
- Checks for `session_id` cookie
- Validates session against database
- Static assets (`.js`, `.css`, `/@vite/`, etc.) bypass auth
- HTML requests redirect to `/` on auth failure
- API requests return 401 Unauthorized on auth failure
- Authenticated requests get `UserContextKey` in context

#### `api/databaseutil`

Database table management and utilities.

**Key Functions:**
- `ExecuteStatement(db *sql.DB, stmt string) error` - Execute DDL/DML
- Table creation and management helpers
- Migration utilities

**Usage:**
```go
import "github.com/chendingplano/shared/go/api/databaseutil"

// Execute table creation
stmt := "CREATE TABLE IF NOT EXISTS..."
err := databaseutil.ExecuteStatement(ApiTypes.PG_DB_miner, stmt)
```

#### `api/RequestHandlers`

Common HTTP request handlers for database operations.

**Files:**
- `DbUtilsCommon.go` - Shared database utilities
- `DbUtilsPG.go` - PostgreSQL-specific handlers
- `DbUtilsMySQL.go` - MySQL-specific handlers
- `HandleJimoRequest.go` - Generic request handler
- `RequestContextPocket.go` - PocketBase context adapter

**Pattern:**
```go
// Generic query/insert/update/delete handlers
// Supports both PostgreSQL and MySQL
// Uses RequestContext abstraction
```

#### `api/EchoFactory`

Factory for creating framework-agnostic request contexts.

**Usage:**
```go
import "github.com/chendingplano/shared/go/api/EchoFactory"

func handler(c echo.Context) error {
    // Create request context wrapper
    rc := EchoFactory.NewFromEcho(c, "HANDLER_LOC_001")

    // Use framework-agnostic methods
    userInfo, err := rc.IsAuthenticated()
    body := rc.GetBody()

    return rc.JSON(200, map[string]interface{}{
        "status": "ok",
    })
}
```

#### `api/sysdatastores`

System-level data stores for users, sessions, etc.

**Common functions:**
- `GetUserInfoByEmail(rc RequestContext, email string) (*UserInfo, error)`
- `GetUserInfoByUserID(rc RequestContext, userID string) (*UserInfo, error)`
- Session management functions

### Configuration (`libconfig.toml`)

The shared library reads configuration from `libconfig.toml`:

```toml
id_start_value = 10000
id_inc_value = 1000
allow_dynamic_tables = true

[system_table_names]
table_name_users = "users"
table_name_login_sessions = "login_sessions"
table_name_activity_log = "activity_log"
# ... more tables

[system_ids]
activity_log_id = "IDs for activity log"
# ... more ID definitions
```

**Loading config:**
```go
// Config is loaded automatically into ApiTypes.LibConfig
tableName := ApiTypes.LibConfig.SystemTableNames.TableNameUsers
```

### Database Support

**Dual Database Support:**
- PostgreSQL (primary, recommended)
- MySQL (legacy support)

**Database Type Constants:**
```go
const (
    PgName    = "postgres"
    MysqlName = "mysql"
)

// Check database type
dbType := ApiTypes.GetDBType()
```

**Connection Pools:**
```go
// Access appropriate pool based on db type
var db *sql.DB
switch ApiTypes.DatabaseInfo.DBType {
case ApiTypes.PgName:
    db = ApiTypes.PG_DB_miner
case ApiTypes.MysqlName:
    db = ApiTypes.MySql_DB_miner
}
```

## 4. Svelte Library (`shared/svelte`)

### Package Information

**Package:** `@chendingplano/shared`
**Version:** 0.0.1
**Type:** ESM module
**Svelte Version:** 5.x (using runes)

### Building and Publishing

```bash
cd shared/svelte

# Build the library
npm run build
# OR
bun run build

# Package for publishing
npm pack

# Run tests
npm test
```

**Output:** Built library in `dist/` directory

### Core Exports

#### Authentication

**`stores/auth.svelte.ts`**

Svelte 5 authentication store with reactive state.

```typescript
import { appAuthStore } from '@chendingplano/shared';
import type { LoginResults } from '@chendingplano/shared';

// Usage in Svelte components
const { user, isAuthenticated, login, logout } = appAuthStore;

// Check auth status
if ($isAuthenticated) {
  console.log('User:', $user);
}

// Login
const result: LoginResults = await login(email, password);

// Logout
await logout();
```

**`utils/auth.ts`**

Utility functions for authentication:

```typescript
import {
  isAuthenticated,
  setIsAuthenticated,
  clearAuthCache,
  setEmailVerifyUrl,
  getEmailVerifyUrl
} from '@chendingplano/shared/auth';

// Check if user is authenticated
const authed = await isAuthenticated();

// Set verification URL
setEmailVerifyUrl('/verify-email');
```

**`components/EmailVerifyPage.svelte`**

Pre-built email verification component:

```svelte
<script lang="ts">
  import { EmailVerifyPage } from '@chendingplano/shared';
</script>

<EmailVerifyPage />
```

#### Database Store and Query Builders

**`stores/dbstore.ts`**

Powerful database store with reactive queries:

```typescript
import { db_store } from '@chendingplano/shared';

// Query data
const users = await db_store.query({
  table_name: 'users',
  field_names: ['id', 'email', 'first_name'],
  condition: { /* ... */ },
  page_size: 20
});

// Insert records
await db_store.insert({
  table_name: 'users',
  records: [{ email: 'test@example.com', first_name: 'Test' }],
  field_defs: [/* ... */]
});

// Update records
await db_store.update({
  table_name: 'users',
  condition: { /* ... */ },
  record: { first_name: 'Updated' },
  field_defs: [/* ... */]
});

// Delete records
await db_store.delete({
  table_name: 'users',
  condition: { /* ... */ },
  field_defs: [/* ... */]
});
```

**Query Builder Utilities:**

```typescript
import {
  cond_builder,
  join_builder,
  update_builder,
  orderby_builder,
  query_builder
} from '@chendingplano/shared';

// Build conditions
const condition = cond_builder
  .atomic('email', '=', 'test@example.com', 'string')
  .and()
  .atomic('verified', '=', true, 'boolean')
  .build();

// Build joins
const joinDef = join_builder
  .from('users')
  .leftJoin('profiles')
  .on('users.id', 'profiles.user_id', '=', 'string')
  .selectFields(['profiles.bio', 'profiles.avatar'])
  .build();

// Build order by
const orderBy = orderby_builder
  .add('created_at', 'timestamp', false) // DESC
  .add('email', 'string', true)          // ASC
  .build();

// Full query builder
const query = query_builder
  .table('users')
  .fields(['id', 'email', 'first_name'])
  .where(condition)
  .orderBy(orderBy)
  .page(0, 25)
  .build();
```

#### In-Memory Stores

**`stores/InMemStores.ts`**

Client-side in-memory data stores:

```typescript
import { GetStoreByName, StoreMap } from '@chendingplano/shared';
import type { RecordInfo, InMemStoreDef } from '@chendingplano/shared';

// Get or create store
const userStore = GetStoreByName<UserRecord>('users');

// Add record
userStore.addRecord({ id: '1', name: 'Test' });

// Get all records
const allUsers = userStore.getAllRecords();

// Find by ID
const user = userStore.getRecordById('1');

// Update record
userStore.updateRecord('1', { name: 'Updated' });

// Delete record
userStore.deleteRecord('1');
```

#### Type Definitions

**`types/CommonTypes.ts`**

**CRITICAL: Types in this file are synchronized with Go types in `shared/go/api/ApiTypes/ApiTypes.go`**

When you see comments like:
```typescript
// Make sure it syncs with ApiTypes.go::FieldDef
export type FieldDef = { ... }
```

**This means:**
1. The TypeScript type MUST match the Go struct exactly
2. Changes to Go types require updating TypeScript types
3. Changes to TypeScript types require updating Go types
4. Field names, types, and JSON tags must align

**Key synchronized types:**
- `FieldDef` - Field definition
- `CondDef` - Query condition
- `OrderbyDef` - Order by clause
- `OnClauseDef` - Join ON clause
- `JoinDef` - Join definition
- `UpdateDef` - Update operation
- `UserInfo` - User information
- `JimoRequest`, `JimoResponse` - Request/response types
- `QueryRequest`, `InsertRequest`, `UpdateRequest`, `DeleteRequest`

**Usage:**
```typescript
import type {
  FieldDef,
  CondDef,
  UserInfo,
  JimoResponse
} from '@chendingplano/shared';

const fields: FieldDef[] = [
  { field_name: 'email', data_type: 'string', required: true, read_only: false }
];
```

**`types/DatabaseTypes.ts`**

Database-specific type definitions.

#### Utility Functions

**`utils/UtilFuncs.ts`**

```typescript
import {
  SafeJsonParseAsObject,
  ParseObjectOrArray,
  GetAllKeys,
  IsValidNonEmptyString,
  ParseConfigFile
} from '@chendingplano/shared';

// Safe JSON parsing
const obj = SafeJsonParseAsObject<MyType>(jsonString);

// Parse flexible input
const data = ParseObjectOrArray(input);

// Get all object keys
const keys = GetAllKeys(object);

// Validate strings
if (IsValidNonEmptyString(value)) {
  // value is non-empty string
}

// Parse TOML config
const config = ParseConfigFile(tomlContent);
```

### Component Architecture

The Svelte library uses **Svelte 5** with runes (`$state`, `$derived`, `$effect`).

**When creating new components:**
- Use `<script lang="ts">` for TypeScript
- Use Svelte 5 rune syntax
- Follow DOM-style events: `onclick`, `oninput` (NOT `on:click`)
- Export components from `src/lib/index.ts`

### Documentation

The `stores/` directory contains comprehensive documentation:

- `Doc-ARCHITECTURE.md` - Architecture overview
- `Doc-README.md` - Main documentation
- `Doc-QUICK_REFERENCE.md` - Quick reference guide
- `Doc-DRIZZLE_COMPARISON.md` - Comparison with Drizzle ORM

**Read these files** when working with the database store and query builders.

## 5. Type Synchronization (CRITICAL)

**⚠️ IMPORTANT: The Go and TypeScript type systems are synchronized manually.**

### How It Works

1. **Go defines the source of truth** in `shared/go/api/ApiTypes/ApiTypes.go`
2. **TypeScript mirrors** these types in `shared/svelte/src/lib/types/CommonTypes.ts`
3. **Comments indicate sync points:**
   ```go
   // Go
   // Make sure it syncs with svelte/src/lib/types/CommonTypes.ts::FieldDef
   type FieldDef struct { ... }
   ```
   ```typescript
   // TypeScript
   // Make sure it syncs with ApiTypes.go::FieldDef
   export type FieldDef = { ... }
   ```

### When to Synchronize

**✅ ALWAYS synchronize when:**
- Adding new fields to a type
- Removing fields from a type
- Changing field types
- Renaming fields
- Changing JSON tags (they become TypeScript property names)

**Types requiring synchronization:**
- `FieldDef`
- `CondDef`
- `OrderbyDef`
- `OnClauseDef`
- `JoinDef`
- `UpdateDef`
- `UserInfo`
- `JimoRequest`
- `JimoResponse`
- `QueryRequest`, `InsertRequest`, `UpdateRequest`, `DeleteRequest`

### Synchronization Checklist

When modifying a synchronized type:

1. **Modify Go struct** in `ApiTypes.go`
2. **Update TypeScript type** in `CommonTypes.ts`
3. **Check JSON tags** match TypeScript property names
4. **Run tests** in both Go and TypeScript
5. **Build both libraries** to verify compatibility
6. **Update all consuming projects** (tax, ChenWeb, deepdoc)

### Example Synchronization

```go
// Go: shared/go/api/ApiTypes/ApiTypes.go
// Make sure it syncs with CommonTypes.ts::FieldDef
type FieldDef struct {
    FieldName   string `json:"field_name"`
    DataType    string `json:"data_type"`
    Required    bool   `json:"required"`
    ReadOnly    bool   `json:"read_only"`
    ElementType string `json:"element_type,omitempty"`
    Desc        string `json:"desc,omitempty"`
}
```

```typescript
// TypeScript: shared/svelte/src/lib/types/CommonTypes.ts
// Make sure it syncs with ApiTypes.go::FieldDef
export type FieldDef = {
  field_name: string;
  data_type: string;
  required: boolean;
  read_only: boolean;
  element_type?: string;
  desc?: string;
};
```

**Note:** Go JSON tags (e.g., `json:"field_name"`) become TypeScript property names (e.g., `field_name`).

## 6. Development Workflow

### Working on Go Library

```bash
cd shared/go

# Run tests
go test ./...

# Format code
go fmt ./...

# Check for issues
go vet ./...

# After changes, sync workspace
cd ../..
go work sync
```

### Working on Svelte Library

```bash
cd shared/svelte

# Install dependencies
bun install

# Build library
bun run build

# Run tests
bun test

# Check types
bun run check

# Format code
bun run format
```

### Testing Changes in Projects

After modifying the shared library, test in dependent projects:

```bash
# Example: Test in tax project
cd ../tax

# For Go changes
go work sync
mise build-server

# For Svelte changes
cd web
bun install  # Picks up local shared library via workspace
mise dev
```

## 7. Common Patterns

### Database Operations

**Go Backend:**
```go
import "github.com/chendingplano/shared/go/api/ApiTypes"

func handler(rc ApiTypes.RequestContext) error {
    db := ApiTypes.PG_DB_miner

    stmt := "SELECT * FROM users WHERE email = $1"
    row := db.QueryRow(stmt, email)

    // ... scan results
}
```

**Svelte Frontend:**
```typescript
import { db_store } from '@chendingplano/shared';

const users = await db_store.query({
  table_name: 'users',
  field_names: ['id', 'email', 'first_name'],
  condition: cond_builder.atomic('verified', '=', true, 'boolean').build()
});
```

### Logging
- Logging with traceability is extremely important!
- Always use ApiTypes.JimoLogger (shared/go/ApiTypes/ApiTypes.go)

```go
"github.com/chendingplano/shared/go/api/loggerutil"

func myfunc() {
    var logger = loggerutil.CreateDefaultLogger(loc)
}
```
ApiTypes.JimoLog is compatible with log/slog.

'loc' specifies the location where the logger is created. It is a statically generated string in the format of "LOC_MMDDHHMMSS", where 'MM' is the month, 'DD' is day, 'HH' is hour, 'MM' is minute, 'SS' is the second. All are 0-padded two-digit strings.

### Authentication Flow

**Go Backend:**
```go
import (
    "github.com/chendingplano/shared/go/authmiddleware"
    "github.com/chendingplano/shared/go/api/EchoFactory"
)

func main() {
    authmiddleware.Init()
    e := echo.New()

    // Protected routes
    api := e.Group("/api")
    api.Use(authmiddleware.AuthMiddleware)
    api.GET("/profile", getProfile)
}

func getProfile(c echo.Context) error {
    rc := EchoFactory.NewFromEcho(c, "PROFILE_001")
    userInfo, _ := rc.IsAuthenticated()

    return rc.JSON(200, map[string]interface{}{
        "user": userInfo,
    })
}
```

**Svelte Frontend:**
```svelte
<script lang="ts">
  import { appAuthStore } from '@chendingplano/shared';

  const { user, isAuthenticated, login } = appAuthStore;

  async function handleLogin() {
    const result = await login(email, password);
    if (result.success) {
      // Logged in
    }
  }
</script>

{#if $isAuthenticated}
  <p>Welcome, {$user?.first_name}!</p>
{:else}
  <button onclick={handleLogin}>Login</button>
{/if}
```

## 8. Versioning and Publishing

### Go Library

The Go library is versioned via git tags and Go modules:

```bash
cd shared/go

# Create version tag
git tag v0.1.0
git push origin v0.1.0

# Consuming projects reference by tag or commit hash
# See go.mod in tax/, ChenWeb/, etc.
```

### Svelte Library

The Svelte library can be published to npm:

```bash
cd shared/svelte

# Update version in package.json
# Build and publish
npm run prepack
npm publish
```

**Current usage:** Projects use the local version via package manager workspace features.

## 9. Migration Notes

### From Application to Shared Library

When moving code from a project to the shared library:

1. **Identify truly shared code** - Used by 2+ projects
2. **Move to appropriate package:**
   - Backend logic → `shared/go/api/`
   - Svelte components → `shared/svelte/src/lib/components/`
   - Types → Sync between Go and TypeScript
3. **Update imports** in all consuming projects
4. **Test in all projects** before committing
5. **Document** the new shared functionality

### Breaking Changes

When making breaking changes to the shared library:

1. **Coordinate with all projects** - Tax, ChenWeb, deepdoc, etc.
2. **Update all consumers** in a single PR/commit
3. **Consider backwards compatibility** when possible
4. **Version appropriately** (major version bump)
5. **Document migration path** in commit message

## 10. Best Practices

### For Go Library

- **Keep connection pools global** - `PG_DB_miner`, `MySql_DB_miner`
- **Use RequestContext abstraction** - Framework-agnostic
- **Log with structured logging** - Use interface ApiTypes.JimoLogger, implemented by `loggerutil.JimoLogger`
- **Handle both databases** - PostgreSQL and MySQL (when applicable)
- **Return rich errors** - Use `fmt.Errorf("context: %w", err)`

### For Svelte Library

- **Use Svelte 5 runes** - `$state`, `$derived`, `$effect`
- **Export from index.ts** - All public APIs
- **Type everything** - Use TypeScript for all code
- **Document stores** - Include usage examples
- **Test query builders** - Ensure SQL generation is correct

### Type Synchronization

- **Go is source of truth** - Define types in Go first
- **Check comments** - Look for "Make sure it syncs with..."
- **Update both sides** - Go struct + TypeScript type
- **Test JSON marshaling** - Ensure Go JSON matches TypeScript
- **Version together** - Type changes affect both libraries

## 11. Troubleshooting

### "Cannot find module '@chendingplano/shared'"

**Solution:**
```bash
cd shared/svelte
bun run build

cd ../../tax/web  # Or other project
bun install
```

### "Undefined exported member 'SomeType'"

**Likely cause:** Type not exported from `shared/svelte/src/lib/index.ts`

**Solution:**
```typescript
// Add to shared/svelte/src/lib/index.ts
export type { SomeType } from './types/CommonTypes';
```

### Go import cycle with authmiddleware

**Cause:** `authmiddleware` uses `EchoFactory.DefaultAuthenticator` to break cycles

**Solution:** Always call `authmiddleware.Init()` before using the middleware:
```go
func main() {
    authmiddleware.Init() // MUST call this first
    // ... rest of setup
}
```

### Type mismatch between Go and TypeScript

**Symptoms:** JSON unmarshaling fails, unexpected null values

**Solution:**
1. Check synchronized type comments in both files
2. Verify JSON tags match TypeScript property names
3. Ensure optional fields use `omitempty` in Go and `?` in TypeScript
4. Test with actual JSON payload

### Database pool not initialized

**Error:** `panic: runtime error: invalid memory address`

**Cause:** Trying to use `ApiTypes.PG_DB_miner` before initialization

**Solution:** Ensure database is initialized before use:
```go
// In main.go or init function
ApiTypes.PG_DB_miner = initPostgresConnection()
```

## 12. See Also

- **Workspace CLAUDE.md** - `/Users/cding/Workspace/CLAUDE.md` for overall workspace structure
- **Tax Project** - `tax/CLAUDE.md` for the primary application using this library
- **Go Module Docs** - Run `go doc` in `shared/go` for package documentation
- **Svelte Store Docs** - See `shared/svelte/src/lib/stores/Doc-*.md` for detailed guides
