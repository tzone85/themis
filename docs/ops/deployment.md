# Deployment

How to run Themis on a Linux host in production. Three install paths,
verification steps, and a systemd unit example.

Verified against `v0.1.0` on `2026-06-03`.

## TL;DR

```bash
docker run --rm \
  -v /var/lib/themis:/data \
  -p 127.0.0.1:8787:8787 \
  ghcr.io/tzone85/themis:v0.1.0 \
  serve --base /data --addr 0.0.0.0:8787
```

Put a reverse proxy (Caddy, Nginx, Traefik) in front to terminate TLS
and enforce rate limits. Don't expose port 8787 directly.

## Install paths

Three supported paths, in order of recommended preference.

### A. Container image (recommended)

```bash
docker pull ghcr.io/tzone85/themis:v0.1.0

# Verify the image signature before running it.
cosign verify ghcr.io/tzone85/themis:v0.1.0 \
  --certificate-identity-regexp '^https://github.com/tzone85/themis/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Image base is `gcr.io/distroless/static-debian12:nonroot`. No shell,
no package manager, runs as UID:GID 65532:65532. Image size ≈ 25 MB.

### B. Signed binary

```bash
VERSION=v0.1.0
ARCH=linux_x86_64   # or linux_arm64, darwin_arm64, etc.

curl -sLo themis.tar.gz \
  "https://github.com/tzone85/themis/releases/download/${VERSION}/themis_${VERSION#v}_${ARCH}.tar.gz"
curl -sLo checksums.txt     "https://github.com/tzone85/themis/releases/download/${VERSION}/checksums.txt"
curl -sLo checksums.txt.sig "https://github.com/tzone85/themis/releases/download/${VERSION}/checksums.txt.sig"
curl -sLo checksums.txt.pem "https://github.com/tzone85/themis/releases/download/${VERSION}/checksums.txt.pem"

# Verify the checksums file came from a Themis release workflow.
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature   checksums.txt.sig \
  --certificate-identity-regexp '^https://github.com/tzone85/themis/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# Verify your archive matches its line in the now-trusted checksums.txt.
grep "$(basename themis.tar.gz)" checksums.txt | sha256sum -c

tar -xzf themis.tar.gz
sudo install -m 0755 themis /usr/local/bin/themis
themis --version
```

### C. From source (developers only)

```bash
git clone https://github.com/tzone85/themis
cd themis
make build
~/.local/bin/themis --version
```

`go install` works too, but skips the build-info ldflags so
`themis --version` reports `dev`. Use `make build` for anything you'd
deploy.

## Tenant bootstrap

A clean install needs at least one tenant with an admin token, a
catalogue snapshot, and a policy file before anything decides.

```bash
BASE=/var/lib/themis

# Where state lives. Create per-tenant subdirs implicitly via the CLI.
sudo install -d -o 65532 -g 65532 -m 0750 "$BASE"

# 1. Tenant — physically separate from any other tenant under $BASE.
themis tenant init --id acme --base "$BASE"

# 2. Admin token. Print it once; the file copy lives under tenants/acme/.
themis tokens grant --base "$BASE" --tenant acme --role admin \
  --description "ops bootstrap"

# 3. Catalogue snapshot. Replace the path with your EventCatalog tree.
themis catalogue sync --id acme --base "$BASE" \
  --source /path/to/event-catalogue

# 4. Policy. Start with the locked-down template; relax from there.
cp docs/onboarding/cookbook/policies/locked-down.yaml "$BASE/themis.yaml"

# 5. Smoke decide.
themis decide --id acme --base "$BASE" \
  --aichange "$BASE/tenants/acme/aichange/example.json" \
  --policy "$BASE/themis.yaml"
```

## Run as a daemon

### Systemd

```ini
# /etc/systemd/system/themis.service
[Unit]
Description=Themis — compliance gateway for AI-generated code
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=themis
Group=themis
Environment=THEMIS_BASE=/var/lib/themis
ExecStart=/usr/local/bin/themis serve --base ${THEMIS_BASE} --addr 127.0.0.1:8787
Restart=on-failure
RestartSec=5s

# Hardening — none of these are required, all reduce blast radius.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/themis
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
LockPersonality=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native
CapabilityBoundingSet=
AmbientCapabilities=

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --shell /usr/sbin/nologin --home-dir /var/lib/themis themis
sudo install -d -o themis -g themis -m 0750 /var/lib/themis
sudo systemctl daemon-reload
sudo systemctl enable --now themis
sudo systemctl status themis
```

### Docker Compose

```yaml
# /etc/themis/docker-compose.yaml
services:
  themis:
    image: ghcr.io/tzone85/themis:v0.1.0
    command: ["serve", "--base", "/data", "--addr", "0.0.0.0:8787"]
    user: "65532:65532"
    ports:
      - "127.0.0.1:8787:8787"
    volumes:
      - /var/lib/themis:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "/themis", "ledger", "doctor", "--id", "acme", "--base", "/data"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Bind-mount permissions: the host directory must be writable by UID
65532 (distroless `nonroot`). If you see "permission denied", run:

```bash
sudo chown -R 65532:65532 /var/lib/themis
```

The same applies to systemd installs where `User=themis` resolves to a
non-65532 UID — pick whichever is consistent across host and container.

## Reverse proxy

### Caddy

```caddy
themis.example.com {
  reverse_proxy 127.0.0.1:8787
  rate_limit {
    zone all { events 60 window 1m }
  }
}
```

### Nginx

```nginx
server {
  listen 443 ssl http2;
  server_name themis.example.com;

  ssl_certificate     /etc/letsencrypt/live/themis.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/themis.example.com/privkey.pem;

  location / {
    proxy_pass http://127.0.0.1:8787;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }

  limit_req zone=themis burst=20 nodelay;
}
```

## Verify a release

After every upgrade, before flipping traffic:

```bash
# 1. Did we actually upgrade?
themis --version

# 2. Is the ledger intact? Run for every tenant.
themis ledger doctor --id acme --base /var/lib/themis

# 3. Smoke a policy decision.
themis decide --id acme --base /var/lib/themis \
  --aichange tenants/acme/aichange/<latest>.json \
  --policy /var/lib/themis/themis.yaml
```

## Air-gapped install

Themis ships with no required outbound calls at runtime. For
air-gapped sites:

- Mirror the GHCR image into your internal registry.
- Switch the signer to local ed25519 (`--signer ed25519:keyfile`) since
  cosign keyless needs Fulcio.
- Disable the OIDC chain step in `tenants/<id>/auth.yaml` if you only
  use file-based tokens.

See [`docs/onboarding/cookbook/`](../onboarding/cookbook/) recipes:
`locked-down policy`, `OIDC chain`, `custom claim mapping`.

## Related

- [`observability.md`](observability.md) — log format, what to scrape.
- [`backup-restore.md`](backup-restore.md) — base-dir snapshot strategy.
- [`runbook.md`](runbook.md) — common incidents.
- [`SECURITY.md`](../../SECURITY.md) — disclosure path.
