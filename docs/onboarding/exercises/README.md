# Exercises — hands-on Themis

Each exercise has a goal, a starting state, and a check command that
confirms you finished. Work through them in order; later exercises build
on state from earlier ones.

Set up your sandbox once:

```bash
go build -o /tmp/themis ./cmd/themis
export DIR=/tmp/themis-exercises
rm -rf "$DIR"
/tmp/themis tenant init --id acme --base "$DIR"
/tmp/themis catalogue sync --id acme --base "$DIR" \
  --source ./internal/catalogue/testdata/sample
```

---

## Exercise 1 — block on secrets

**Goal:** Make Themis deny a PR that adds an AWS access key to source.

**Starting state:** clean tenant from setup above.

**Steps:**

1. Create an AIChange whose touched file is `src/leak.go`:

   ```bash
   echo '{"pr_id":"ex1#1","actor":"claude_code","touched_files":[
     {"path":"src/leak.go","change_kind":"ADDED","after_hash":"h"}
   ]}' > "$DIR/ai.json"
   ```

2. Create a workdir with the offending file:

   ```bash
   mkdir -p "$DIR/work/src"
   echo 'aws_id = "AKIAIOSFODNN7EXAMPLE"' > "$DIR/work/src/leak.go"
   ```

3. Write a policy that blocks on any secret finding:

   ```bash
   cat > "$DIR/themis.yaml" <<EOF
   version: 1
   default: ALLOW
   rules:
     - name: secrets block
       when:
         findings.kind: secret
       then:
         verdict: DENY
   EOF
   ```

4. Decide:

   ```bash
   /tmp/themis decide --id acme --base "$DIR" \
     --aichange "$DIR/ai.json" --policy "$DIR/themis.yaml" \
     --workdir "$DIR/work"
   ```

**Check:** `verdict` must be `DENY`. The `SCAN_FINDING` event in the
ledger should carry `kind:"secret"` and `severity:"critical"`.

---

## Exercise 2 — require sign-off for schema-breaking changes

**Goal:** A modification to `events/PaymentReceived/schema.json` must
require approval, and granting from a `senior` role finalises the
decision.

**Steps:**

1. AIChange touching the schema:

   ```bash
   echo '{"pr_id":"ex2#1","actor":"claude_code","touched_files":[
     {"path":"events/PaymentReceived/schema.json","change_kind":"MODIFIED",
      "before_hash":"a","after_hash":"b"}
   ]}' > "$DIR/ai.json"
   ```

2. Policy:

   ```bash
   cat > "$DIR/themis.yaml" <<EOF
   version: 1
   default: ALLOW
   rules:
     - name: schema breaking
       when:
         impact.kind: [SCHEMA_BREAKING]
       then:
         verdict: REQUIRE_APPROVAL
         required_approvers:
           - role: senior
   EOF
   ```

3. Decide + grant:

   ```bash
   /tmp/themis decide --id acme --base "$DIR" \
     --aichange "$DIR/ai.json" --policy "$DIR/themis.yaml"

   /tmp/themis approval grant --id acme --base "$DIR" \
     --pr-id "ex2#1" --approver "human:alice" --role senior \
     --comment "reviewed"
   ```

**Check:** the grant output's `finalised:true` and
`final_verdict:"ALLOW"`. A `DECISION_FINALISED` event must be present in
the ledger.

---

## Exercise 3 — emergency override + post-mortem

**Goal:** Override a hypothetical DENY decision and close the
post-mortem.

**Steps:**

1. Invoke:

   ```bash
   /tmp/themis override invoke --id acme --base "$DIR" \
     --pr-id "ex3#1" \
     --actor "human:alice" --co-signer "human:bob" \
     --reason "Catalogue host unreachable from CI — merging logging hotfix to restore traffic."
   ```

2. Confirm status:

   ```bash
   /tmp/themis override status --id acme --base "$DIR" --pr-id "ex3#1"
   ```

3. Close the post-mortem:

   ```bash
   /tmp/themis override close-postmortem --id acme --base "$DIR" \
     --pr-id "ex3#1" \
     --closer "human:compliance" \
     --notes "Catalogue health check timeout was 1s; bumped to 5s + alert."
   ```

**Check:** the close output's `postmortem_closed:true`. Try invoking
with a < 50-character reason — it must error.

---

## Exercise 4 — REST API + role gates

**Goal:** Mint two tokens with different roles and confirm the gates
behave correctly.

**Steps:**

1. Tokens:

   ```bash
   READ_TOK=$(/tmp/themis tokens grant --base "$DIR" \
     --tenant acme --role read --description ex4-read | grep ^thm_)

   ADMIN_TOK=$(/tmp/themis tokens grant --base "$DIR" \
     --tenant acme --role admin --description ex4-admin | grep ^thm_)
   ```

2. Start the server:

   ```bash
   /tmp/themis serve --base "$DIR" --addr 127.0.0.1:8788 &
   sleep 1
   ```

3. Read can GET health; cannot POST anchor:

   ```bash
   curl -s -o /dev/null -w "read GET health: %{http_code}\n" \
     -H "Authorization: Bearer $READ_TOK" \
     http://127.0.0.1:8788/v1/tenants/acme/health

   curl -s -o /dev/null -w "read POST anchor: %{http_code}\n" \
     -X POST -H "Authorization: Bearer $READ_TOK" \
     -H "Content-Type: application/json" -d '{}' \
     http://127.0.0.1:8788/v1/tenants/acme/anchor
   ```

**Check:** the first prints `200`, the second `403`. Repeat with
`$ADMIN_TOK` and both should be `200`.

---

## Exercise 5 — MCP from an agent loop

**Goal:** Drive the MCP bridge against a running server and confirm the
`themis_decisions` tool returns the most recent decision.

**Steps:**

1. With the server from Exercise 4 still running, issue a request:

   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}
   {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{
     "name":"themis_decisions",
     "arguments":{"pr_id":"ex2#1"}
   }}' | /tmp/themis mcp \
        --base-url http://127.0.0.1:8788 \
        --token "$ADMIN_TOK" --tenant-id acme
   ```

**Check:** the second response's `content[0].text` includes
`"verdict":"REQUIRE_APPROVAL"` (or `DECISION_FINALISED → ALLOW` if
Exercise 2 was completed).

---

## Exercise 6 — tamper the ledger and watch the alarm fire

**Goal:** Confirm `themis ledger verify` records `LEDGER_INTEGRITY_BROKEN`
to the sidecar `incidents.jsonl` when the chain is broken.

**Steps:**

1. Flip a byte mid-file:

   ```bash
   python3 -c "
   p='$DIR/tenants/acme/events.jsonl'
   raw=open(p,'rb').read()
   raw=raw[:10]+bytes([raw[10]^1])+raw[11:]
   open(p,'wb').write(raw)
   "
   ```

2. Verify:

   ```bash
   /tmp/themis ledger verify --id acme --base "$DIR" || true
   ```

**Check:** the command exits non-zero, and
`$DIR/tenants/acme/incidents.jsonl` contains a JSON line with
`"kind":"LEDGER_INTEGRITY_BROKEN"`.

---

## Cleanup

When you're done:

```bash
pkill -f 'themis serve' 2>/dev/null
rm -rf "$DIR"
```

Every exercise here mirrors a real test in `internal/`. If you change
the underlying code and any exercise stops working, the corresponding
test should also fail — that's the bridge between the tutorial and the
production guarantees.
