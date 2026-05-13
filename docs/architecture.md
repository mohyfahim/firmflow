# Firmflow Backend Architecture

See also: [HTTP API reference](api.md), [Developer onboarding](onboarding.md), and [Device TCP protocol](device-tcp-protocol.md).

## Architectural Style

- Modular monolith with clear package boundaries under `internal/domain`.
- Thin HTTP handlers, service-layer business logic, and repository data access.
- Constructor-based dependency injection with explicit module wiring in `internal/bootstrap`.
- Future extraction-ready domains by keeping cross-domain access behind interfaces.

## Package Layout

- `cmd/server`: executable entrypoint and graceful shutdown.
- `internal/config`: environment-backed application configuration grouped by app/http/db/auth/mail/storage/rate_limit/**device_ota** (TCP OTA listen address, download token TTL, public base URL for absolute download links).
- `internal/bootstrap`: object graph wiring and application bootstrapping.
- `internal/common`: shared primitives (errors, response envelopes, pagination, validation, UTC helpers).
- `internal/middleware`: request ID, structured logging, recovery, CORS, and centralized API error handling.
- `internal/database`: database bootstrap, naming strategy, pooling, and migration integration hook.
- `internal/platform`: infrastructure adapters (logger, mailer, storage, security, jobs, observability).
- `internal/transport/http`: HTTP routes, handlers, and DTOs.
- `internal/transport/devotcp`: optional **binary TCP** OTA listener (device token auth, poll/report, short-lived download token issuance); see `docs/api.md` and `protocol.go`.
- `internal/domain`: module boundaries for auth, users, projects, roles, devices (including **OTA download token** persistence under `device/model`), firmware, campaigns, audit, dashboard.
- `migrations`: SQL migrations (reserved).
- `test`: integration and E2E tests (reserved).

## Shared Conventions

- UTC-only timestamps across all modules.
- UUID as public-facing identifiers.
- JSON response envelope:
  - success: `{ "data": ..., "meta": ... }`
  - error: `{ "error": { "code", "message", "details?", "request_id" } }`
- Every request carries `X-Request-ID` for tracing and debugging.
- Request-scoped logger is attached by middleware for consistent structured logs.
- Repository pattern for GORM query encapsulation.
- Mandatory authz checks for mutating handlers (project routes use `RequireProjectPermission` with keys from the permission registry).
- Security-by-default: no sensitive secrets/tokens in logs.

## Authentication Domain

- **Sessions**: Refresh tokens are stored hashed; access tokens are short-lived JWTs issued per session (`internal/domain/auth/security`).
- **Middleware**: `RequireAuth` validates `Authorization: Bearer`, parses JWT claims, and sets `auth_user_id` and `auth_session_id` on the Gin context for downstream handlers.
- **Account lifecycle**: Registration with email verification, password reset, optional TOTP-based 2FA with recovery codes, profile CRUD, session listing/revocation, and scheduled account deletion live under `/api/v1/auth` and `/api/v1/me`.

## Projects & RBAC

- **Project workspace**: Each project has metadata (name, description, archive state) and **membership** linking users to roles (`internal/domain/project`, `internal/domain/rbac`).
- **Authorizer**: `AuthorizeProject(projectID, userID, permissionKey)` loads membership and resolves effective permissions (system roles + custom roles). Failed checks return API errors before the handler runs.
- **HTTP layering**: Routes register `RequireAuth` first, then `RequireProjectPermission` with a specific permission constant from `internal/domain/rbac/permission` (for example `project.read`, `member.invite`, `audit.read`).
- **Ownership-sensitive actions**: Some routes (for example ownership transfer) rely on service-layer checks in addition to authentication.
- **Permission registry**: Keys for future modules (devices, firmware, campaigns) are reserved in `permission.All()` so seeds and custom roles stay aligned as those APIs are implemented.

## Evolution Path

The codebase aims at a **modular monolith** today with **Hexagonal-style** boundaries (handlers → services → repositories → models). New domains should add migration hooks, repositories, services, handlers, and route registration without leaking persistence types across packages.
