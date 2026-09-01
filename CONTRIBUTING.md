# Contributing

Thank you for considering a contribution. Bug reports, fixes and improvements are welcome. For
anything that could be exploited, use the private route in [SECURITY.md](SECURITY.md) — never a
public issue.

For anything larger than a small fix, please open an issue first and describe what you want to
change and why. This service writes a permanent evidence trail, so several of its properties are
load-bearing for people who will never read the code, and a change that fights its design is better
redirected before it is written than after.

## Building and testing

You need the Go toolchain at the version named in [go.mod](go.mod). Every dependency is public, so
nothing needs credentials, a `GOPRIVATE` setting or a vendor directory. The gate a change must pass
is the same one CI runs:

```sh
go build ./...
go vet ./...
go test -race -count=1 ./...
```

Three more checks run in CI and are worth running before you push:

- **Lint** — `golangci-lint run`, at the version pinned in
  [.github/workflows/ci.yml](.github/workflows/ci.yml); the repo's [.golangci.yml](.golangci.yml)
  carries the configuration.
- **Vulnerabilities** — `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
- **The image** — CI builds it, generates an SBOM, fails on HIGH/CRITICAL findings from a
  vulnerability scan, and signs it. A change to the [Dockerfile](Dockerfile) should be built
  locally before you push, because that job is slow to fail.

The committed tree must already be tidy: CI runs `go mod tidy -diff` and fails if it would change
anything, so run `go mod tidy` after touching dependencies. All Go code is `gofmt`-formatted, and
`.gitattributes` pins Go files to LF line endings — leave that alone, it keeps the tidy-diff gate
stable across platforms.

Tests need no broker and no database: the in-memory store builds the same chain, and the consumer is
packaged as a task that simply is not started when no broker is configured. If a change makes a test
need a live NATS or PostgreSQL, that is a design signal worth raising in the issue rather than
solving with a fixture.

## What a change to this service needs

Read the **Security invariants** section of the [README](README.md) before changing anything on the
consume or append path. They are not documentation of good intentions; each one is the reason a
specific class of defect cannot happen.

The three that carry the most weight:

- **The canonical bytes are a stored-data contract, not an implementation detail.** Every link in
  every deployed chain was computed as `SHA-256(canonical(envelope) || prev_hash)` over the bytes
  today's encoding produces. Change how an envelope is canonicalised — field order, encoding,
  timestamp handling, which fields are covered — and every already-written chain stops verifying,
  which reads to an auditor as tampering. Such a change is a migration with a re-anchor plan, never
  a refactor, and it needs a test that pins the bytes.
- **A broken link is rejected, never absorbed.** The append verifies continuity against the current
  head and refuses when it does not hold. "Repairing" a mismatch by re-linking, re-hashing or
  skipping ahead destroys the only property the chain has. Rejection followed by redelivery is the
  correct behaviour, including when it is inconvenient under concurrency.
- **Nothing is acknowledged that was not durably appended.** Delivery is at-least-once, and that is
  safe only because a failed append is negatively acknowledged and redelivered while `event_id`
  idempotency keeps a retry from creating a second row. A change that acknowledges early — or that
  swallows an append error — silently truncates an evidence trail nobody will notice is short until
  they need it.

Also load-bearing:

- **Append-only.** The database role holds EXECUTE-only grants and reaches nothing but procedures;
  `UPDATE` and `DELETE` are revoked and guarded, and deletion exists only through a deliberate
  retention purge. A change that opens any other route is the change, not a side effect of one.
- **Both store backends implement the same semantics.** The in-memory backend is what the tests
  prove behaviour against, so a rule that lands in only one of the two is a rule the tests cannot
  see. The PostgreSQL half of a rule may also live in the schema's procedures, which are deployed
  separately from this repository — a domain-rule change that stops at the Go layer is incomplete.
- **The store holds references, not identities.** Subject identifiers are pseudonymous, attributes
  carry no free text and no document content. This record is permanent; anything that lands in it is
  very hard to unland.
- **Boot is loud about its development fallbacks.** No broker means no consumer; no store DSN means a
  non-durable in-memory chain. Keep both noisy — a silent fallback in production is a service that
  looks healthy while recording nothing.

## Proposing a change

- Work on a branch and open a pull request against `develop`. `develop` is merged into `main` and
  tagged there when a release goes out, so `main` is never committed to directly.
- **Sign off every commit.** This project uses the
  [Developer Certificate of Origin](https://developercertificate.org/): by adding a
  `Signed-off-by: Your Name <you@example.org>` line you certify that you wrote the change or
  otherwise have the right to submit it under this project's licence. `git commit -s` adds the line
  for you; the name and address must match the commit author. A pull request whose commits lack it
  fails the DCO check and cannot be merged.
- Keep the change focused: one concern per pull request.
- A change in behaviour comes with a test that fails without it.
- Match the style around you — naming, error handling, comment density. Comments explain what and
  why in plain domain terms; a reference to a standard is cited in the bracket form already used in
  the code.
- A change an operator or an integrator can feel — a new or changed event field, error code,
  configuration knob or default — belongs in [CHANGELOG.md](CHANGELOG.md) in the same pull request.
- Pull requests also run a dependency review. A new dependency needs a reason the standard library
  or the existing ones cannot cover.

## Licence

This project is licensed under the **MIT licence** (see [LICENSE](LICENSE)). By submitting a
contribution you agree that it is provided under the same licence — you keep the copyright in what
you wrote, and everyone, including commercial users, may use it under MIT's terms.
