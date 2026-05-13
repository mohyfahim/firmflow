# Developer onboarding (Firmflow backend)

This document is derived from the current codebase layout and conventions. Read it alongside [HTTP API reference](api.md), [Architecture](architecture.md), and [Device TCP protocol](device-tcp-protocol.md) if you integrate firmware over TCP.

## What you are building

**Firmflow** is a multi-tenant SaaS backend for **MCU / fleet OTA**: projects, RBAC, devices, firmware artifacts, and **rollout campaigns**. Devices authenticate separately from dashboard users (hashed **device tokens**). Firmware bytes live in a pluggable **object store** (local disk in dev).

Primary stack: **Go 1.23**, **Gin**, **GORM**, **PostgreSQL**, **Logrus**.

## Day zero: run the API locally

1. Copy environment: `cp .env.example .env` and adjust secrets (`AUTH_JWT_SECRET`, DB password, etc.).
2. Start stack: `make up` (Docker Compose: API + Postgres) or run Postgres yourself and `make run`.
3. Enable schema sync in dev if you want GORM `AutoMigrate`: `DB_AUTO_MIGRATE=true` in `.env` (see `internal/bootstrap/app.go` for which migrators run).
4. Health: `GET http://localhost:8080/health/live` and `/health/ready`.

Makefile shortcuts: `make tidy`, `make test`, `make fmt`, `make lint` — see repository `README.md`.

## Repository map (where things live)

| Path | Role |
|------|------|
| `cmd/server/main.go` | Starts HTTP server, waits for SIGINT/SIGTERM, calls `bootstrap.New()`, `StopSchedulers()`, `StopOTA()`, `HTTPServer.Shutdown`. |
| `internal/bootstrap/app.go` | Wires **all** dependencies: DB, Gin engine, middleware, domain services, **HTTP routes**, optional **TCP OTA** goroutine (`devotcp.Serve`). |
| `internal/config` | `config.Load()` reads env; includes **Device OTA** (`DEVICE_OTA_TCP_ADDR`, `OTA_DOWNLOAD_TOKEN_TTL`, `PUBLIC_HTTP_BASE_URL`). |
| `internal/database` | Postgres connection + migration runner hook. |
| `internal/middleware` | `RequireAuth` (JWT), `RequireDeviceAuth` (hashed device token), `RequireProjectPermission`, CORS, logging, panic recovery, `ErrorHandler`. |
| `internal/transport/http` | `routes/routes.go` is the **route table**; `handlers/*` are thin Gin handlers calling domain services. |
| `internal/transport/devotcp` | Optional **binary TCP** OTA protocol (`protocol.go`, `handler.go`, `server.go`). **Wire format**: [docs/device-tcp-protocol.md](device-tcp-protocol.md). |
| `internal/domain/*` | One folder per bounded context: `model` (GORM structs + `Migrator` where present), `repository` (GORM queries), `service` (rules + authz orchestration). |
| `internal/common` | API errors (`apperrors`), JSON envelope helpers, pagination, JSON validation helpers. |
| `internal/platform` | Logger, mailer noop, **local object store** under `storage`. |
| `firmflow-bruno/` | Bruno HTTP collection for manual testing (import folder, use `develop` environment). |

Cross-domain rule of thumb: **handlers** never talk to another domain’s repository directly; they go through **services** declared in bootstrap.

## Request lifecycle (HTTP)

1. Gin receives request → global middleware (request ID, logger, recovery, CORS, error handler).
2. Route group applies `RequireAuth` and/or `RequireDeviceAuth` or stays public.
3. Project-scoped routes use `RequireProjectPermission(authorizer, permissionKey)` from `internal/domain/rbac/permission`.
4. Handler parses params/DTO, calls **service** method with `context.Context` and actor IDs.
5. Service calls **repository** / other services, returns domain errors wrapped as `apperrors` where needed.
6. `middleware.ErrorHandler` maps errors to HTTP status + JSON envelope `{ "error": { "code", "message", "details?", "request_id" } }`.

Canonical permission strings: `internal/domain/rbac/permission/registry.go`.

## Two authentication worlds

| Audience | Header | Validated in |
|----------|--------|----------------|
| Dashboard / API users | `Authorization: Bearer <JWT>` | `middleware.RequireAuth` → `internal/domain/auth/service` |
| Field devices | `Authorization: Device <raw_token>` | `middleware.RequireDeviceAuth` → `device/repository.GetDeviceByActiveTokenHash` (stores **SHA-256 hash** of raw token only) |

Blocked devices and revoked tokens are rejected at middleware (HTTP) or the same lookup (TCP). Device-facing JSON routes: `POST /api/v1/device/poll`, `POST /api/v1/device/report` (`internal/transport/http/routes/routes.go`).

## OTA data flow (high level)

1. **Campaigns** (`internal/domain/campaign`) create **assignments** per device when a rollout is active.
2. **Poll** (HTTP or TCP) moves **pending → offered** once, and can keep returning OTA metadata while **offered** (see `campaign/repository` `FindActivePendingOffer` and `service.BuildPollOffer`).
3. **TCP path** (`devotcp.Handler`): after poll, issues a **short-lived OTA download token** (`firmware/service/IssueOtaDownloadToken`, row in `ota_download_tokens` via `device/repository`).
4. Device calls **`GET /api/v1/device/firmware-download?token=...`** (no JWT); `OpenFirmwareWithOtaDownloadToken` consumes the token and streams bytes from the object store (`firmware/service/ota_download.go`).
5. **Report** updates assignment progress (`campaign/service/ota_report.go` → `ApplyOtaDeviceReport`); HTTP report still uses version-match install detection where applicable.

Background **campaign scheduler**: `campaignsvc.RunScheduler` in bootstrap (30s tick); stops on `StopSchedulers()`.

## Database and models

- GORM models live under each domain’s `model/` package; composite `Migrator` types implement `database.Migrator` and are listed in `bootstrap` when `DB_AUTO_MIGRATE` is true.
- Device rows are in `project/model` stubs (`Device` struct) for historical layout; device-specific tables (types, auth tokens, connection logs, **OTA download tokens**) live under `internal/domain/device/model`.
- Prefer **UTC** `time.Time` in services; the codebase assumes UTC for timestamps.

## Testing

- Run full suite: `go test ./...` from repo root.
- Domain tests often use **SQLite in-memory** with the same `Migrator` chain as production models (see `*_test.go` files under `internal/domain/.../service`).
- TCP framing tests: `internal/transport/devotcp/protocol_test.go`.

When adding features: extend the smallest surface (repository method + service + handler + route) and add a focused test in the same domain package if patterns already exist there.

## Bruno collection (`firmflow-bruno/`)

1. Open [Bruno](https://www.usebruno.com/), **Open Collection** → select the `firmflow-bruno` directory.
2. Select environment **`develop`** (`environments/develop.bru`).
3. Run **Auth → Login**; copy `data.access_token` into collection/env var `access_token`.
4. Set `project_id`, `device_id`, `firmware_id`, etc., from prior responses (register device → `device_token` for device poll/report).

Folders: `auth`, `me`, `health`, `projects`, `devices`, `firmware`, **`campaigns`**, **`device-ota`** (anonymous download by token).

## Common tasks (where to edit)

| Task | Start here |
|------|----------------|
| New HTTP route | `internal/transport/http/routes/routes.go`, new handler method, wire in `internal/bootstrap/app.go` if new deps. |
| New permission | `internal/domain/rbac/permission/registry.go` + seeds if needed (`rbac/model/seed.go`). |
| Device / campaign rule change | `internal/domain/campaign/service` or `device/service`; SQL-heavy bits in `campaign/repository`. |
| OTA binary protocol change | `internal/transport/devotcp/protocol.go` + tests; keep `handler.go` orchestration thin. |
| New env var | `internal/config/config.go` + `.env.example` + this doc if user-facing. |
| API documentation | `docs/api.md` (tables); link from `README.md` if major. |

## Security reminders for new code

- Never log raw device tokens or OTA download tokens; persist only **hashes** (see `internal/domain/auth/security/token.go`).
- OTA download tokens are **short-lived** and **one-time**; do not reuse them across devices.
- User JWT secret must be strong in production (`AUTH_JWT_SECRET`).

## Further reading

- [docs/api.md](api.md) — full route list and bodies.
- [docs/architecture.md](architecture.md) — design intent and package boundaries.
- [docs/device-tcp-protocol.md](device-tcp-protocol.md) — TCP OTA wire format for firmware clients.
- `README.md` — Make targets and quick start.

If something in this file drifts from code, treat the **source of truth** as `internal/bootstrap/app.go` and `internal/transport/http/routes/routes.go`.
