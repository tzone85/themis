# Runbook

Common incidents, diagnosis steps, and remediations. Each entry:
symptom → quick check → fix → escalation.

Verified against `v0.1.0` on `2026-06-03`.

## Incident index

1. [`ledger doctor` reports hash mismatch](#1-ledger-doctor-reports-hash-mismatch)
2. [OIDC chain fallback firing constantly](#2-oidc-chain-fallback-firing-constantly)
3. [Anchor sink unreachable](#3-anchor-sink-unreachable)
4. [BOM signer failing](#4-bom-signer-failing)
5. [Heartbeat checker stuck](#5-heartbeat-checker-stuck)
6. [`themis serve` won't bind to port](#6-themis-serve-wont-bind-to-port)
7. [Tenant directory permission denied](#7-tenant-directory-permission-denied)
8. [Policy decision unexpectedly REQUIRE_APPROVAL](#8-policy-decision-unexpectedly-require_approval)

---

## 1. `ledger doctor` reports hash mismatch

**Symptom.** `themis ledger doctor --id <tenant> --base <base>` exits
non-zero with `hash mismatch at seq=N`.

**What it means.** The append-only ledger has been edited — either an
event line was rewritten, deleted, or inserted out of order. This is a
trust-critical event: it implies tampering or filesystem corruption.

**Quick check.**

```bash
# 1. Identify the exact event that breaks the chain.
themis ledger doctor --id <tenant> --base <base>

# 2. Inspect events around seq=N in the JSONL file.
awk -v n=N 'NR>=n-2 && NR<=n+2' "$BASE/tenants/<tenant>/ledger.jsonl"

# 3. Compare against your last good snapshot.
diff <(head -n N "$BASE/tenants/<tenant>/ledger.jsonl") \
     <(head -n N /backup/.../tenants/<tenant>/ledger.jsonl)
```

**Fix.**

- **Filesystem corruption (single bad block).** Restore the ledger
  from the most recent verified snapshot — see
  [`backup-restore.md`](backup-restore.md). The events written *after*
  the last good snapshot must be re-ingested from source PRs.
- **Editing by a privileged user.** The audit trail is broken; treat
  this as a security incident. Capture the bad file, restore from
  backup, rotate any tokens that could have authored the edit, and
  see [`SECURITY.md`](../../SECURITY.md).

**Escalation.** Always page security on a hash mismatch that isn't
trivially explained by a known-bad disk.

## 2. OIDC chain fallback firing constantly

**Symptom.** Every auth attempt logs "OIDC store returned error,
falling back to file store" (or equivalent) — or auth simply succeeds
for callers who shouldn't have access.

**What it means.** `internal/auth.ChainStore` is composing
`OIDCTokenStore` + `FileTokenStore`. A *non-401* error from the IdP
should short-circuit; only a clean 401 falls through. If you see
fall-through on every call, your IdP is unreachable and you've
configured the chain in a permissive order.

**Quick check.**

```bash
# 1. Test the IdP endpoint directly.
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $TOKEN" \
  "$IDP_USERINFO_URL"

# 2. Re-read the auth config.
cat "$BASE/tenants/<tenant>/auth.yaml"
```

**Fix.**

- IdP unreachable for network reasons → fix the network; until then,
  the file store keeps existing operators working.
- Config order wrong → put `OIDCTokenStore` *first* in the chain so
  IdP-issued tokens take precedence; `FileTokenStore` second as the
  break-glass for ops.
- `CacheTTL` too high → IdP rotated keys but Themis still caches the
  old answer. Drop `CacheTTL` ≤ 60s for production.

**Escalation.** None unless tokens that should be denied are being
allowed; then security.

## 3. Anchor sink unreachable

**Symptom.** `themis ledger anchor --id <tenant> --base <base> --sink
s3://...` exits non-zero with a network error.

**What it means.** The external anchor (Plan 11) records a Merkle root
to a third party (S3, IPFS, etc.) for independent verification. If the
sink is down, anchoring stops but local ledger integrity is unaffected.

**Quick check.**

```bash
# 1. Test the sink reachable from the host.
aws s3 ls s3://<bucket>/ --region <region>

# 2. Confirm credentials available where themis runs.
sudo -u themis env | grep -i aws

# 3. Last successful anchor:
themis ledger anchor --id <tenant> --base <base> --list | head -5
```

**Fix.**

- Backoff and retry — `themis ledger anchor` is idempotent; the next
  run picks up from the last anchored seq.
- Credentials missing in the daemon's env → add them to the systemd
  unit's `Environment=` or to the Docker Compose `environment:`.

**Escalation.** Sink down > 24h → page on-call to confirm a fallback
sink (a different region / a different provider).

## 4. BOM signer failing

**Symptom.** `themis bom sign --id <tenant> --base <base> --pr-id <id>`
exits non-zero.

**What it means.** Either the configured signer can't reach its
dependencies (cosign keyless needs Fulcio; ed25519 needs the key
file), or the BOM was tampered with after `bom build`.

**Quick check.**

```bash
# Which signer is configured?
themis --version
grep signer "$BASE/tenants/<tenant>/auth.yaml" || \
  grep signer "$BASE/themis.yaml"

# Reproduce with verbose flags.
themis bom sign --id <tenant> --base <base> --pr-id <id> --signer ed25519:<keyfile>
```

**Fix.**

- **cosign keyless, no OIDC token in env.** Run from a CI step with
  the right `id-token` permissions, or switch to ed25519 for the local
  path.
- **ed25519, key file missing.** Restore the key file from your
  secret store. Rotate if it was deleted.
- **BOM hash mismatch.** Rebuild the BOM (`themis bom build`) then
  re-sign. The build step recomputes from the AIChange that triggered
  the decision.

**Escalation.** Lost ed25519 key in production → security; rotate +
re-anchor.

## 5. Heartbeat checker stuck

**Symptom.** `themis heartbeat watch` runs but `HEARTBEAT_FAIL` events
keep landing in the ledger, or no `HEARTBEAT_OK` for > 2 ticks.

**What it means.** The polling daemon (Plan 16) detects whether a
PR-ingesting adapter (e.g. `git_heuristic`) has been silenced. A stuck
heartbeat means the adapter source is unreachable or the ingestion
side is dead.

**Quick check.**

```bash
# 1. Run a one-shot check to surface the immediate error.
themis heartbeat run-once --id <tenant> --base <base>

# 2. Inspect the last heartbeat events.
tail -n 20 "$BASE/tenants/<tenant>/ledger.jsonl" | grep HEARTBEAT
```

**Fix.**

- Adapter source unreachable → fix the network / VPN / SSH key on the
  ingestion side.
- Adapter misconfigured → reload the adapter config and re-run
  `heartbeat run-once`.

**Escalation.** Heartbeat fails > 1 hour → page on-call; AI changes
may be flowing through with no audit capture.

## 6. `themis serve` won't bind to port

**Symptom.** `listen tcp 127.0.0.1:8787: bind: address already in use`.

**Fix.**

```bash
sudo ss -ltnp | grep 8787       # who's holding it?
sudo systemctl stop themis      # or kill the other process
sudo systemctl start themis
```

If running under Docker, `docker ps | grep 8787`.

## 7. Tenant directory permission denied

**Symptom.** `open tenants/acme/ledger.jsonl: permission denied`.

**Fix.**

```bash
# Container: nonroot UID 65532 must own the volume.
sudo chown -R 65532:65532 /var/lib/themis

# Systemd: themis user (or whichever User= you set) must own it.
sudo chown -R themis:themis /var/lib/themis
sudo chmod 0750 /var/lib/themis
```

The two cases are mutually exclusive; pick one runtime model per
host.

## 8. Policy decision unexpectedly REQUIRE_APPROVAL

**Symptom.** A PR that "should" pass is gated.

**Quick check.**

```bash
# Re-run decide with explicit inputs.
themis decide --id <tenant> --base <base> \
  --aichange tenants/<tenant>/aichange/<pr>.json \
  --policy <base>/themis.yaml \
  --explain
```

`--explain` prints which rule fired and why. The `default:` clause is
the fallback — if no rule allowed, the default wins.

**Fix.**

- Add a more specific rule above `default`.
- Or update the catalogue if the entity classification was the gate
  (`themis catalogue sync` then re-decide).

**Escalation.** None — this is by design.

---

## When to escalate

| Symptom | Page |
|---|---|
| Hash mismatch on the ledger | Security |
| Lost signer key | Security |
| OIDC fall-through allowing forbidden access | Security |
| Anchor sink down > 24h | On-call |
| Heartbeat failing > 1h | On-call |
| Daemon won't start | On-call |
| Wrong policy verdict | None — it's policy, fix the rule |

## Related

- [`deployment.md`](deployment.md) — install + bootstrap.
- [`observability.md`](observability.md) — what to scrape.
- [`backup-restore.md`](backup-restore.md) — recovery procedures.
- [`SECURITY.md`](../../SECURITY.md) — escalation channel.
