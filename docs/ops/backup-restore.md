# Backup & restore

Themis is a stateful single-binary service. State is a directory tree
on disk per tenant. Backup strategy follows from that.

Verified against `v0.1.0` on `2026-06-03`.

## What's on disk

The `--base` directory passed to `themis serve` holds everything:

```
<base>/
├── themis.yaml                    # global policy (operator-edited)
└── tenants/
    └── <tenant-id>/
        ├── ledger.jsonl           # append-only Merkle-chained event log
        ├── ledger.sqlite          # SQLite WAL projection of ledger
        ├── ledger.sqlite-wal      # WAL — must be snapshotted with main
        ├── ledger.sqlite-shm      # shared-memory; ephemeral
        ├── catalogue/             # EventCatalog snapshot
        ├── aichange/              # per-PR AIChange JSON
        ├── bom/                   # per-PR AI-BOMs (and signatures)
        ├── mempalace/drawers/     # advisor memory drawers
        ├── tokens/                # admin/operator API tokens
        ├── auth.yaml              # OIDC chain config (optional)
        └── overrides/             # operator override records
```

The single most important file is `ledger.jsonl`. Lose it and the
audit trail is gone. The SQLite projection can be rebuilt from it.

## Snapshot strategy

### What to snapshot

Everything under `<base>/` *except* `cache/` and `tmp/`. Atomicity
matters for the SQLite files — snapshot WAL + main together.

### Daily — full

```bash
BASE=/var/lib/themis
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
DEST=/backup/themis/$STAMP

# Quiesce SQLite — checkpoint WAL into the main file before snapshot.
for db in "$BASE"/tenants/*/ledger.sqlite; do
  sqlite3 "$db" "PRAGMA wal_checkpoint(TRUNCATE);"
done

# Snapshot. Use --one-file-system to avoid following bind mounts.
sudo rsync -a --one-file-system \
  --exclude 'cache/' --exclude 'tmp/' \
  "$BASE"/ "$DEST"/

# Verify ledger integrity on the snapshot before declaring success.
for tenant in $(ls "$DEST/tenants"); do
  themis ledger doctor --id "$tenant" --base "$DEST" \
    || { echo "FAIL: ledger doctor on $tenant"; exit 1; }
done
```

### Continuous — ledger only

Tail `tenants/*/ledger.jsonl` and ship every appended line to an
external object store. Themis's append-only invariant means this is
safe to do with no locking. Replay from `ledger.jsonl` rebuilds the
SQLite projection.

```bash
# Example with vector.
[sources.themis_ledgers]
type = "file"
include = ["/var/lib/themis/tenants/*/ledger.jsonl"]
read_from = "end"

[sinks.s3]
type = "aws_s3"
inputs = ["themis_ledgers"]
bucket = "themis-audit-trail"
key_prefix = "tenants/{{ host }}/"
encoding.codec = "ndjson"
```

## Restore

### From a daily snapshot

```bash
NEW_BASE=/var/lib/themis
SNAPSHOT=/backup/themis/20260603T040000Z

sudo systemctl stop themis
sudo rsync -a --delete "$SNAPSHOT"/ "$NEW_BASE"/

# Owner must match the runtime UID (65532 for distroless,
# whatever you picked for systemd).
sudo chown -R 65532:65532 "$NEW_BASE"

# Verify *before* serving traffic.
for tenant in $(ls "$NEW_BASE/tenants"); do
  themis ledger doctor --id "$tenant" --base "$NEW_BASE"
done

sudo systemctl start themis
```

### From `ledger.jsonl` only

If the SQLite projection is missing or corrupt but `ledger.jsonl`
survived:

```bash
themis ledger doctor --id <tenant> --base <base> --rebuild
```

This rebuilds `ledger.sqlite` from `ledger.jsonl` and verifies the
hash chain end-to-end. Cost: O(events) — minutes per million.

## What you *cannot* restore from

- A `ledger.jsonl` that has been edited mid-file. The hash chain
  breaks; `themis ledger doctor` will report the exact event where
  the chain diverges.
- A tenant directory that was overwritten by another tenant's data.
  Cross-tenant restore is rejected by design — paths under
  `tenants/<id>/` are physically isolated.
- BOM signatures whose key material was rotated. If you signed with
  cosign keyless, the cert is bound to a specific GH Actions run; if
  you signed with local ed25519 and lost the key, the signature is
  uncheckable. Rotate carefully.

## Drill

Once a quarter, restore to a clean host from the latest snapshot and
run:

```bash
themis ledger doctor --id <tenant> --base <restored-base>
themis decide --id <tenant> --base <restored-base> \
  --aichange tenants/<tenant>/aichange/<latest>.json \
  --policy <restored-base>/themis.yaml
```

Both must exit zero. If they don't, treat the backup as untrusted and
fix the gap.

## Retention

- **Ledger events**: forever. They are the audit trail; compliance
  buyers pay for the property that nothing falls off.
- **Catalogue snapshots**: 30 days of dailies, then monthlies for a
  year. Older catalogues rarely need restoring.
- **AIChange JSON / BOM JSON**: forever; small, append-only.
- **MemPalace drawers**: forever; small, content-addressed.

## Related

- [`deployment.md`](deployment.md) — install paths and tenant
  bootstrap.
- [`runbook.md`](runbook.md) — what to do when `ledger doctor` fails.
- Design spec §ledger model.
