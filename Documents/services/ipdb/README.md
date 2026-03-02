# IPDB Service — IP Geolocation via ip66.dev

IP geolocation lookup powered by the [ip66.dev](https://ip66.dev/) free MMDB database.
Provides ASN, country, and continent data for any IPv4 or IPv6 address with
PostgreSQL-backed caching and daily automated sync.

---

## Overview

| Item | Detail |
|------|--------|
| Source | https://ip66.dev/ (Cloud 66, CC BY 4.0) |
| Database format | MaxMind DB (MMDB) |
| Data | ASN number + org, country name + ISO, continent name + code |
| Update frequency | Daily (background goroutine) |
| Cache | PostgreSQL (`ipdb_lookup_cache`), configurable TTL |
| Go package | `github.com/chendingplano/shared/go/api/ipdb` |

---

## Package Layout

```
shared/go/api/ipdb/
├── types.go    – IPRecord, SyncStatus, and internal mmdbRecord structs
├── store.go    – PostgreSQL table creation, cache read/write, sync log
├── lookup.go   – MMDB reader (hot-swappable), LookupIP entrypoint
└── sync.go     – Download scheduler, atomic file replacement, Init/Shutdown

shared/go/api/RequestHandlers/
└── ipdb_handlers.go  – Echo HTTP handlers (same pattern as icons_handlers.go)
```

---

## Initialisation

Call `ipdb.Init(logger)` **once** at application startup, after the database
connection pools are ready and `sysdatastores.CreateSysTables` has run.
Call `ipdb.Shutdown()` during graceful shutdown.

```go
// In main.go (or equivalent startup sequence):
sysdatastores.CreateSysTables(logger)   // creates ipdb_lookup_cache & ipdb_sync_log
ipdb.Init(logger)                        // loads MMDB, starts 24 h sync loop
defer ipdb.Shutdown()
```

`sysdatastores.CreateSysTables` calls `ipdb.CreateTables(logger)` automatically,
so no separate table-creation step is needed.

---

## Configuration (environment variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `IPDB_FILE_PATH` | `/var/data/ip66.mmdb` | Local path where the MMDB file is stored |
| `IPDB_CACHE_TTL_DAYS` | `7` | Days before a cached lookup is considered stale |

---

## API Endpoints

All endpoints require authentication (`Authorization` cookie / header).

### `GET /shared_api/v1/ipdb/lookup`

Look up a single IP address.

**Query parameters**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `ip` | yes | IPv4 or IPv6 address |

**Example**

```bash
curl -b "session_id=<token>" \
  "https://api.example.com/shared_api/v1/ipdb/lookup?ip=8.8.8.8"
```

**Response**

```json
{
  "Status": true,
  "ResultType": "json",
  "NumRecords": 1,
  "Results": {
    "ip": "8.8.8.8",
    "asn_number": 15169,
    "asn_org": "GOOGLE",
    "country_name": "United States",
    "country_iso": "US",
    "continent_name": "North America",
    "continent_code": "NA",
    "looked_up_at": "2026-03-03T12:00:00Z"
  }
}
```

---

### `GET /shared_api/v1/ipdb/sync/status`

Returns the most recent sync log entry.

**Response**

```json
{
  "Status": true,
  "Results": {
    "id": "...",
    "status": "success",
    "file_size": 52428800,
    "synced_at": "2026-03-03T00:00:00Z"
  }
}
```

---

### `POST /shared_api/v1/ipdb/sync/trigger` *(admin only)*

Triggers an immediate background sync. Returns `202 Accepted` right away;
the download runs asynchronously.

```bash
curl -X POST -b "session_id=<admin-token>" \
  "https://api.example.com/shared_api/v1/ipdb/sync/trigger"
```

---

## Database Tables

### `ipdb_lookup_cache`

Caches recent IP lookups. Stale entries (older than `IPDB_CACHE_TTL_DAYS`) are
purged automatically after every successful sync.

| Column | Type | Notes |
|--------|------|-------|
| `ip` | `VARCHAR(45) PK` | Supports IPv6 |
| `asn_number` | `BIGINT` | |
| `asn_org` | `VARCHAR(256)` | |
| `country_name` | `VARCHAR(128)` | |
| `country_iso` | `VARCHAR(10)` | ISO 3166-1 alpha-2 |
| `continent_name` | `VARCHAR(64)` | |
| `continent_code` | `VARCHAR(10)` | |
| `looked_up_at` | `TIMESTAMPTZ` | Refreshed on every cache hit |
| `created_at` | `TIMESTAMPTZ` | |

### `ipdb_sync_log`

Audit trail of every sync attempt (success and failure).

| Column | Type | Notes |
|--------|------|-------|
| `id` | `VARCHAR(40) PK` | UUID |
| `status` | `VARCHAR(20)` | `"success"` or `"failure"` |
| `file_size` | `BIGINT` | Bytes downloaded |
| `error_msg` | `TEXT` | Non-empty on failure |
| `synced_at` | `TIMESTAMPTZ` | |

---

## Sync Mechanism

1. On `ipdb.Init`, if `/var/data/ip66.mmdb` (or `IPDB_FILE_PATH`) exists it is
   opened immediately so lookups are available without waiting for a download.
2. If the file does not exist, a synchronous initial sync runs before the server
   starts accepting traffic.
3. A background goroutine fires every 24 hours:
   - Downloads `https://downloads.ip66.dev/db/ip66.mmdb` to a `.tmp` file.
   - Atomically renames the temp file to the final path (no mid-file reads).
   - Hot-swaps the in-memory MMDB reader (zero downtime, no restart needed).
   - Purges stale cache entries so re-lookups reflect the new data.
   - Writes a record to `ipdb_sync_log`.
4. Only one sync can run at a time; concurrent calls are skipped.
5. Manual sync can be triggered via `POST /shared_api/v1/ipdb/sync/trigger`
   (admin only, runs in background).

---

## Location Codes

| Range | Area |
|-------|------|
| `SHD_IPD_0XX` | General / types |
| `SHD_IPD_1XX` | Store / database |
| `SHD_IPD_2XX` | MMDB lookup |
| `SHD_IPD_3XX` | Sync |
| `SHD_IPD_4XX` | HTTP handlers |

---

## Attribution

The ip66.dev database is published by [Cloud 66](https://cloud66.com) under the
[Creative Commons Attribution 4.0 International (CC BY 4.0)](https://creativecommons.org/licenses/by/4.0/) licence.
