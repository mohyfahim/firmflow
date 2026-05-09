# Firmflow OTA Backend

Production-grade backend foundation for a SaaS OTA platform (microcontrollers), built with Go, Gin, GORM, and PostgreSQL.

## Tech Stack

- Go
- Gin
- GORM
- PostgreSQL
- Logrus (structured logging)

## Current Scope

This scaffold includes:

- environment-based config loading grouped by concerns:
  - `app`, `http`, `db`, `auth`, `mail`, `storage`, `rate_limit`
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
- modular domain layout with implemented **authentication**, **current-user account** APIs (`/api/v1/auth`, `/api/v1/me`), and **projects + project RBAC** (`/api/v1/projects`, `/api/v1/projects/:projectID/...`)

Further surfaces (device registry, firmware artifacts, OTA campaigns, fleet dashboards) are represented by packages and permission keys but not yet wired as HTTP handlers.

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

### Projects & RBAC (implemented)

Multi-tenant workspaces are modeled as **projects**. Authenticated users create projects, invite members, assign roles, and manage custom roles with permission keys from the central registry (`project.read`, `member.invite`, `device.read`, etc.). Routes under `/api/v1/projects/:projectID` enforce **project-scoped RBAC** via middleware that checks membership and permission keys before the handler runs.

### Authentication for API clients

Protected routes expect:

```http
Authorization: Bearer <access_token>
```

Login and refresh return a `TokenPair` inside the JSON envelope (`data.access_token`, `data.refresh_token`, `data.session_id`, …). Use the access token until it expires, then call `POST /api/v1/auth/refresh` with the refresh token.

### HTTP API reference

See [docs/api.md](docs/api.md) for method/path summaries, request bodies, and common query parameters.

### Bruno collection

The [Bruno](https://www.usebruno.com/) API collection lives in `firmflow-bruno/`. Import the folder as a collection, select the `develop` environment, run **Login**, then paste `data.access_token` (and optionally `data.refresh_token`, project UUIDs) into the environment variables so **me** and **projects** requests authenticate.

### Roadmap (skeleton domains)

Device inventory, firmware artifacts, OTA campaigns, and fleet dashboards are modeled in package layout and permission keys but not yet exposed as HTTP APIs. They will plug into the same config, middleware, and RBAC patterns as auth and projects.
