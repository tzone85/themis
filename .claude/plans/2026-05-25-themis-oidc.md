# Plan 18 — OIDC TokenStore + ChainStore

**Date:** 2026-05-25
**Depends on:** Plan 12 (auth + roles).
**Status:** ✅ Shipped — see commit `eeff948` ("feat(auth): OIDCTokenStore +
ChainStore — pluggable IdP behind same TokenStore interface").
**Scope:** Replace + augment the file-backed `TokenStore` from Plan 12
with an OIDC userinfo-driven implementation, plus a `ChainStore` that
composes multiple stores in precedence order so file + OIDC can coexist
in production.

## Tasks

### T1 — `OIDCTokenStore`

`internal/auth/oidc.go`. Validates Bearer tokens by calling an IdP's
`/userinfo` endpoint. Configurable `HTTPClient` (so tests substitute
httptest), `CacheTTL` (in-memory cache so a busy ramp doesn't hammer the
IdP), `ClaimMapper` (so any IdP claim scheme can drive `(tenant, role)`).

### T2 — `DefaultClaimMapper`

Reads `tenant` + `role` + `description` claims directly. Production
deployments override it (e.g. derive tenant from a group membership claim).

### T3 — `ChainStore`

Composes multiple `TokenStore`s. First successful lookup wins;
`ErrUnauthorized` from one store falls through to the next; any *other*
error short-circuits so an IdP outage can't silently weaken access
control.

### T4 — Tests

12 unit tests cover IdP happy path, 401/5xx mapping, missing-URL guard,
default + custom claim mappers, cache hits + TTL expiry, chain precedence
+ fall-through + short-circuit + empty-chain default.

### T5 — Docs + ci

README Plan-18 changelog entry; `make ci` green.

## Definition of done

- [x] Bearer tokens validated against a live IdP userinfo URL.
- [x] Claim mapping is pluggable.
- [x] `ChainStore(FileTokenStore, OIDCTokenStore)` lets file + OIDC coexist.
- [x] Cache invalidation on logout deferred — short `CacheTTL` is the operational guidance until an introspection-revocation hook ships.
