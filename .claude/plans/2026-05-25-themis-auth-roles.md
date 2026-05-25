# Plan 12 — Multi-tenant API-key Resolution + Role Model

**Date:** 2026-05-25
**Depends on:** Plans 1-11.
**Scope:** Replace the flat per-tenant `api-tokens` file with a structured
token store that maps each token to `(tenant_id, role)`. Wire a role-aware
middleware into the REST surface so write endpoints require elevated
roles, and lay the interface OIDC will plug into later.

## Role model (design spec §6.1)

```
read       — GET endpoints only (dashboards, audit queries)
dev        — read + POST /decide   (developer pre-merge checks)
reviewer   — dev + POST /approvals (named approvers)
compliance — reviewer + POST /override + close-postmortem
admin      — compliance + POST /anchor + POST /heartbeat + tenant admin
```

Roles are ordered: a token with `compliance` satisfies any rule requiring
`reviewer`, `dev`, or `read`.

## Token store

New file: `tenants/tokens.yaml` (at base, shared across tenants):

```yaml
tokens:
  - token: "abc123..."
    tenant: "acme"
    role: "dev"
    description: "alice's laptop"
  - token: "xyz789..."
    tenant: "acme"
    role: "admin"
```

Backwards-compatibility: the existing per-tenant `tenants/<id>/api-tokens`
file still works — tokens listed there are treated as `admin` for that tenant.

## Tasks

### T1: `internal/auth` package

- `Role` enum + `Rank()` ordering.
- `Identity { Tenant, Role, Token (last 4 chars only), Description }`.
- `TokenStore` interface: `Lookup(token string) (Identity, error)`.
- `FileTokenStore` impl reading `tokens.yaml` + falling back to per-tenant `api-tokens` files.
- `ErrUnauthorized`, `ErrInsufficientRole`.

### T2: `RequireIdentity(base, r, minRole)` middleware

Replaces `RequireToken`; returns the resolved Identity or an error.

### T3: Wire role requirements into existing API endpoints

Per the table above. Keep backwards compatibility: when only the legacy
`api-tokens` file exists, requests work as before (every token = admin).

### T4: `themis tokens` CLI

- `themis tokens grant --tenant <t> --role <r> --description <d>` — generates a random token, appends to `tokens.yaml`, prints it once.
- `themis tokens list` — shows all tokens (last 4 chars + tenant + role + description).
- `themis tokens revoke --token-prefix <p>` — removes matching entry.

### T5: Tests

- Unit tests for role ordering + token-store resolution.
- Integration tests for role-gated endpoints (read-token gets 403 on POST).
- Backwards-compat test: a legacy `api-tokens` file still works.

### T6: README Plan 12 changelog

### T7: `make ci` pass
