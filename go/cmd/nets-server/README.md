# nets-server

`nets-server` is a small CLI wrapper that runs an embedded `nats-server` (with JetStream support) from Go.

## What It Does

- Starts a NATS server process
- Enables JetStream by default
- Supports auth via `NATS_TOKEN` or `NATS_USER`/`NATS_PASS`
- Handles graceful shutdown on `SIGINT`/`SIGTERM`

## Run

From `shared/go`:

```bash
go run ./cmd/nets-server
```

Or from this directory:

```bash
go run .
```

## Run With mise

From `shared/go/cmd/nets-server`:

```bash
mise run nets-server-run
```

Available tasks:

- `mise run nets-server-run`
- `mise run nets-server-run-public`
- `mise run nets-server-run-token` (uses `NATS_TOKEN`)
- `mise run nets-server-run-userpass` (uses `NATS_USER` and `NATS_PASS`)
- `mise run nets-server-build`
- `mise run nets-server-test`

## Configuration

The command supports environment variables and equivalent CLI flags.

| Env Var | Flag | Default | Description |
|---|---|---|---|
| `NATS_HOST` | `--host` | `127.0.0.1` | NATS listen host |
| `NATS_PORT` | `--port` | `4222` | NATS client port |
| `NATS_HTTP_PORT` | `--http-port` | `8222` | Monitoring port (`0` disables) |
| `NATS_STORE_DIR` | `--store-dir` | `nats-data` | JetStream storage directory |
| `NATS_JETSTREAM` | `--jetstream` | `true` | Enable JetStream |
| `NATS_USER` | `--user` | empty | Username auth (must pair with `NATS_PASS`) |
| `NATS_PASS` | `--pass` | empty | Password auth (must pair with `NATS_USER`) |
| `NATS_TOKEN` | `--token` | empty | Token auth (mutually exclusive with user/pass) |

## Examples

Start with defaults:

```bash
go run .
```

Start with explicit host/ports:

```bash
go run . --host 0.0.0.0 --port 4222 --http-port 8222
```

Start with token auth:

```bash
NATS_TOKEN=my-token go run .
```

Start with username/password auth:

```bash
NATS_USER=app NATS_PASS=secret go run .
```

Disable JetStream:

```bash
go run . --jetstream=false
```

## Monitoring

By default, monitoring endpoints are exposed on `http://127.0.0.1:8222`.

Common endpoints:

- `/varz`
- `/connz`
- `/routez`
- `/jsz` (JetStream stats)

## Test

```bash
go test .
```
