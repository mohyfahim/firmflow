# Firmflow OTA Backend

Production-grade backend foundation for a SaaS OTA platform (microcontrollers), built with Go, Gin, GORM, and PostgreSQL.

## Tech Stack

- Go
- Gin
- GORM
- PostgreSQL
- Logrus (structured logging)

## Current Scope

The repo includes:

- environment-based config loading grouped by concerns:
  - `app`, `http`, `db`, `auth`, `mail`, `storage`, `rate_limit`, **device OTA** (`DEVICE_OTA_TCP_ADDR`, `OTA_DOWNLOAD_TOKEN_TTL`, `PUBLIC_HTTP_BASE_URL` — see `.env.example`)
- dependency bootstrap
- Gin server setup
- middleware stack (request ID, request-scoped logger, logging, recovery, CORS, centralized errors)
- PostgreSQL connection bootstrap with GORM naming strategy and pool tuning
- migration integration hook (`internal/database/migrator.go`)
- health endpoints:
  - `GET /health/live`
  - `GET /health/ready`
- graceful shutdown
- Dockerfile for app image
- Docker Compose for local app + PostgreSQL
- Makefile targets
- modular domain layout with implemented:
  - **Authentication** and **current-user** APIs (`/api/v1/auth`, `/api/v1/me`)
  - **Projects** and **project-scoped RBAC** (`/api/v1/projects`, `/api/v1/projects/:projectID/...`)
  - **Custom roles** (list with catalog, CRUD) and **members** under each project
  - **Devices**: device types (catalog + custom), device groups, filtered device list, registration, twin, block/unblock, token rotation, **bulk** actions, and **device-facing** poll/report with `Authorization: Device <token>`
  - **Firmware**: multipart upload (SHA-256, size limits), metadata + compatible device types, version/semver handling, **local object storage** (swappable for S3-style backends), authenticated **download** stream (no public URLs)
  - **OTA campaigns**: create/list/detail with firmware + group/explicit targets, **time_scheduled** / **immediate** / **stable percentage** rollouts, pause/resume/cancel, **per-device progress**, device **poll/report** integration for offers and install detection; **background scheduler** activates due campaigns (30s tick, graceful stop on shutdown); optional **binary TCP OTA** (`internal/transport/devotcp`) with **short-lived download tokens** and **`GET /api/v1/device/firmware-download?token=...`**

Richer fleet analytics and dashboards are still ahead; see `docs/api.md` for API detail and **`docs/onboarding.md`** for a code-oriented walkthrough for new contributors.

## Documentation

- [docs/api.md](docs/api.md) — HTTP routes and device/OTA behavior.
- [docs/architecture.md](docs/architecture.md) — layering and package roles.
- [docs/onboarding.md](docs/onboarding.md) — onboarding from the codebase (where to change things, auth, OTA flows, Bruno).

## Local Development

1. Copy env file:

   ```bash
   cp .env.example .env
   ```

2. Start app + PostgreSQL:

   ```bash
   make up
   ```

3. Download dependencies:

   ```bash
   make tidy
   ```

4. Run API server:

   ```bash
   make run
   ```

5. Test health endpoints:

   ```bash
   curl http://localhost:8080/health/live
   curl http://localhost:8080/health/ready
   ```

## Make Targets

- `make setup` - copy `.env` if missing and tidy modules
- `make up` - start local app + PostgreSQL
- `make down` - stop local stack
- `make build` - build app image
- `make run` - run API server
- `make test` - run tests
- `make lint` - run go vet
- `make fmt` - go fmt
- `make tidy` - go mod tidy
- `make migrate` - run migration hook (`DB_AUTO_MIGRATE=true`)

## Auth API (Implemented)

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/verify-email`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/forgot-password`
- `POST /api/v1/auth/reset-password`
- `GET /api/v1/me/profile`
- `PATCH /api/v1/me/profile`
- `POST /api/v1/me/change-password`
- `GET /api/v1/me/sessions`
- `DELETE /api/v1/me/sessions/:sessionID`
- `DELETE /api/v1/me/sessions/others`
- `POST /api/v1/me/2fa/enable`
- `POST /api/v1/me/2fa/confirm`
- `POST /api/v1/me/2fa/disable`
- `DELETE /api/v1/me`

### Projects, RBAC, and devices (implemented)

Multi-tenant workspaces are **projects**. Under `/api/v1/projects/:projectID`, middleware checks membership and a **permission key** per route (e.g. `project.read`, `member.invite`, `device.read`, `firmware.read`, `firmware.upload`, `campaign.read`, `campaign.create`). Devices live in a project; **device auth** for field agents uses a separate header on `/api/v1/device/*` (see [docs/api.md](docs/api.md)).

### Authentication for API clients

Protected routes expect:

```http
Authorization: Bearer <access_token>
```

Login and refresh return a `TokenPair` inside the JSON envelope (`data.access_token`, `data.refresh_token`, `data.session_id`, …). Use the access token until it expires, then call `POST /api/v1/auth/refresh` with the refresh token.

### HTTP API reference

See [docs/api.md](docs/api.md) for method/path summaries, request bodies, and common query parameters.

### Bruno collection

The [Bruno](https://www.usebruno.com/) API collection lives in `firmflow-bruno/`. Import the folder as a collection, select the **`develop`** environment, run **Login**, then set:

- `access_token` (from `data.access_token`)
- `project_id` (from create/list project responses)
- For device flows: `device_type_id`, `device_id`, `group_id`, and after **Register device** or **Rotate token**, `device_token` for **Device poll** / **Device report** (`Authorization: Device …` is set in those requests).
- For firmware: `firmware_id` after upload/list; multipart **Upload firmware** uses form fields `version`, `changelog`, `device_type_ids` (JSON array string), and `file`.
- For campaigns: `campaign_id` after **campaigns-create** (or from list response).
- For **anonymous firmware download** (OTA token): set `ota_download_token` to the 64-character hex token from a TCP poll response (Bruno request **firmware-download-by-token** under **device-ota**).

Folders mirror major areas: `auth`, `me`, `health`, `projects`, `devices`, `firmware`, **`campaigns`**, **`device-ota`**.

### Roadmap

Deeper fleet analytics, richer TCP report semantics, and multi-phase percentage waves may follow; they should extend the same layering and RBAC patterns. See `docs/onboarding.md` for where to plug in changes.
