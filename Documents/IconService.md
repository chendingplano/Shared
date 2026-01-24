## Icon Service

### Basic Info
- Initial Creator: Claude Code
- Created at: 2026/01/23

### Initial Request
I plan to have a Icon service, implemented in the 'shared/' directory. The main features of this service include:
- Store icon files in the directory ${DATA_HOME_DIR}/icons/<icon_category/
- A Svelte component that lets users pick icons
- A Svelte component that view, add, and delete icons


## Icon Service Implementation Plan
### Overview
Implement a reusable Icon service in the shared/ directory that provides:

- File-based icon storage at ${DATA_HOME_DIR}/icons/<category>/
- Go backend with API endpoints for icon CRUD operations
- Svelte components for icon picking and management

### Directory Structure
Go Backend (shared/go/)

```text
shared/go/api/
├── icons/ 
│   ├── icons_types.go        # Type definitions (IconDef, requests/responses)
│   ├── icons_service.go      # Service interface and implementation
│   └── icons_handlers.go     # Echo HTTP handlers
├── sysdatastores/
│   └── table-icons.go        # Database table creation and CRUD
└── router.go                 # (modify to add icon routes)
```
Svelte Frontend (shared/svelte/src/lib/)

```text
├── components/icons/
│   ├── IconPicker.svelte     # Component for selecting icons
│   ├── IconManager.svelte    # Admin component for view/add/delete
│   ├── IconGrid.svelte       # Reusable icon grid display
│   └── index.ts              # Export barrel
├── stores/
│   └── iconStore.ts          # API client functions
├── types/
│   └── IconTypes.ts          # TypeScript types (synced with Go)
└── index.ts                  # (modify to export new components)
```

### Database Schema

```sql
CREATE TABLE IF NOT EXISTS icons (
    id              VARCHAR(40) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name            VARCHAR(128) NOT NULL,
    category        VARCHAR(64) NOT NULL,
    file_name       VARCHAR(255) NOT NULL,
    file_path       VARCHAR(512) NOT NULL,
    mime_type       VARCHAR(64) NOT NULL,
    file_size       BIGINT NOT NULL DEFAULT 0,
    width           INTEGER DEFAULT NULL,
    height          INTEGER DEFAULT NULL,
    tags            JSONB DEFAULT '[]',
    description     TEXT DEFAULT NULL,
    creator         VARCHAR(64) NOT NULL,
    updater         VARCHAR(64) NOT NULL,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT uq_icon_category_name UNIQUE (category, name),
    CONSTRAINT chk_mime_type CHECK (mime_type IN ('image/svg+xml', 'image/png', 'image/jpeg', 'image/webp', 'image/gif'))
);

CREATE INDEX IF NOT EXISTS idx_icons_category ON icons (category);
CREATE INDEX IF NOT EXISTS idx_icons_name ON icons (name);
```

### API Endpoints
| Method | Route | Description Auth |
|:-------|:------|:-----------------|
|GET	| /shared_api/v1/icons	| List icons (with filters)	Yes
|GET	| /shared_api/v1/icons/:id	| Get icon metadata	Yes
|GET	| /shared_api/v1/icons/file/:category/:filename	| Serve icon file	Yes
|POST	| /shared_api/v1/icons	| Upload icon (multipart)	Admin
|DELETE	| /shared_api/v1/icons/:id	| Delete icon	Admin
|GET	| /shared_api/v1/icons/categories	| List categories	Yes

### Design Decisions:

- All endpoints require authentication (session cookie)
- Categories are user-created (no predefined list)
- Supported formats: SVG, PNG, JPEG, WebP, GIF

## Component Design
### IconPicker.svelte
- Purpose: Let users select an icon from available options
- Props: selected, onSelect, allowedCategories
- Features: Search by name/tags, filter by category, grid display

### IconManager.svelte
- Purpose: Admin UI to view, add, and delete icons
- Features:
    - View all icons grouped by category
    - Upload new icons with name, category (user-created), tags
    - Delete existing icons with confirmation
    - File type validation (SVG, PNG, JPEG, WebP, GIF)
    - Create new categories on-the-fly during upload

### IconGrid.svelte
Purpose: Reusable grid display of icons
Props: icons, selectedId, onSelect, showDelete, onDelete

## Implementation Phases
### Phase 1: Database & Types
1. Create shared/go/api/icons/icons_types.go - Go type definitions
2. Create shared/go/api/sysdatastores/table-icons.go - Table creation and CRUD
3. Create shared/svelte/src/lib/types/IconTypes.ts - TypeScript types

### Phase 2: Backend Service & Handlers
1. Create shared/go/api/icons/icons_service.go - Service with dependency injection
1. Create shared/go/api/icons/icons_handlers.go - Echo HTTP handlers
1. Modify shared/go/api/router.go - Register icon routes

### Phase 3: Frontend API Client
1. Create shared/svelte/src/lib/stores/iconStore.ts - API client class
1. Export from shared/svelte/src/lib/index.ts

### Phase 4: Svelte Components
1. Create IconGrid.svelte - Base grid component
1. Create IconPicker.svelte - Selection component
1. Create IconManager.svelte - Admin CRUD component
1. Export components from index files

## Key Files to Modify/Create
### New files:

- shared/go/api/icons/icons_types.go
- shared/go/api/icons/icons_service.go
- shared/go/api/icons/icons_handlers.go
- shared/go/api/sysdatastores/table-icons.go
- shared/svelte/src/lib/types/IconTypes.ts
- shared/svelte/src/lib/stores/iconStore.ts
- shared/svelte/src/lib/components/icons/IconPicker.svelte
- shared/svelte/src/lib/components/icons/IconManager.svelte
- shared/svelte/src/lib/components/icons/IconGrid.svelte
- shared/svelte/src/lib/components/icons/index.ts

### Modified files:

- shared/go/api/router.go - Add icon routes
- shared/svelte/src/lib/index.ts - Export new components/types

## Environment Configuration

```text
DATA_HOME_DIR=/path/to/data   # Base directory for icon storage
# Icons stored at: ${DATA_HOME_DIR}/icons/<category>/<filename>
```

### Todos

- Create shared/go/api/icons/icons_types.go - Go type definitions

- Create shared/go/api/sysdatastores/table-icons.go - Table creation and CRUD

- Create shared/svelte/src/lib/types/IconTypes.ts - TypeScript types

- Create shared/go/api/icons/icons_service.go - Service with dependency injection

- Create shared/go/api/icons/icons_handlers.go - Echo HTTP handlers

- Modify shared/go/api/router.go - Register icon routes

- Create shared/svelte/src/lib/stores/iconStore.ts - API client

- Create IconGrid.svelte - Base grid component

- Create IconPicker.svelte - Selection component

- Create IconManager.svelte - Admin CRUD component

- Export components from index files

## Implementation Summary
### Backend (Go) - shared/go/
#### New Files:

- icons_types.go - Type aliases to ApiTypes
- icons_service.go - File storage service with dependency injection
- icons_handlers.go - Echo HTTP handlers
- table-icons.go - Database CRUD operations

#### Modified Files:

- ApiTypes.go - Added IconDef, IconUploadRequest, IconUpdateRequest, IconListRequest types
- router.go - Registered icon routes under /shared_api/v1/icons

### Frontend (Svelte) - shared/svelte/
#### New Files:

- IconTypes.ts - TypeScript type definitions
- iconStore.ts - API client class
- IconGrid.svelte - Reusable icon grid display
- IconPicker.svelte - Icon selection component
- IconManager.svelte - Admin CRUD component
- index.ts - Component barrel export

#### Modified Files:

- index.ts - Exported new components, types, and iconStore

### API Endpoints
| Method | Route | Description |
|:-------|:------|:------------|
| GET	| /shared_api/v1/icons	| List icons with filters |
| GET	| /shared_api/v1/icons/:id	| Get icon by ID |
| GET	| /shared_api/v1/icons/categories	| List categories |
| POST	| /shared_api/v1/icons	| Upload icon (admin) |
| DELETE	| /shared_api/v1/icons/:id	| Delete icon (admin) |
| GET	| /shared_api/v1/icons/file/:category/:filename	| Serve icon file

### To Use in Projects

1. Initialize the service in your app's startup:
```text
icons.InitIconService(os.Getenv("DATA_HOME_DIR"))
```

2. Create the database table:
```text
sysdatastores.CreateIconsTable(logger, db, "pg", "icons")
```

3. Import Svelte components:
```text
import { IconPicker, IconManager, iconStore } from '@chendingplano/shared';
```
