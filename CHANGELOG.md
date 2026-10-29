# Changelog

Notable changes to this service, newest first, per release. This file is written for whoever
runs the service or integrates against it.

## Unreleased

### Added

- **`EIDAS_AUDIT_STORE_DSN_FILE`.** The database connection string carries the role's password, so it can now be
  read from a file (for example one a secrets agent writes). `EIDAS_AUDIT_STORE_DSN`, when set, still wins, so an
  existing deployment changes nothing. If the file is named but missing or empty, the service refuses to start
  instead of falling back to the non-durable in-memory store.

### Changed

- **Libraries updated, and the service now builds with Go 1.27.3:** azugo v0.40.2, go-platform-kit v1.11.5 and
  go-sec-events v1.2.3, with the other dependencies at their current releases. Nothing about the API, the
  configuration or the database changes.

## v0.1.2

A security release. **The service now builds with Go 1.27.2**, and its libraries move to their Go
1.27.2 releases. Nothing about the API, the configuration or the database changes.

### Changed

- **Go 1.27.2** (was 1.27.0), both in `go.mod` and as the image's default build toolchain
  (`ARG GO_VERSION` in the `Dockerfile`). Go 1.27.2 and `golang.org/x/net` v0.60.0 fix
  vulnerabilities that this service's code reaches under 1.27.0: `govulncheck` found nine it calls,
  GO-2026-6603, -6605, -6607, -6608, -6610, -6611, -6612, -6613 and -6617, in `net/http`,
  `crypto/tls`, `net/textproto` and `golang.org/x/net`. Under the new set it finds none. **Rebuild
  the image to pick the fixes up**; an image built before this change still carries them.
- **Libraries:** `go-platform-kit` → v1.11.4, `go-sec-events` → v1.2.2, azugo → v0.40.0, `fasthttp`
  → v1.75.0, plus 39 indirect modules with them. None of azugo v0.40.0's behaviour changes reaches
  this service: it sets no cookies, issues no redirects after a non-GET request, makes no outbound
  calls through azugo's HTTP client and implements no azugo cache.

### Notes

- The image build now caches Go modules and compiled packages between builds. The binary is built by
  the same command with the same flags; only the build's layer order and caches changed.
- CI's linter moved to golangci-lint v2.14.0. The earlier release cannot read Go 1.27.2's compiled
  standard library and stops before linting anything.
- The gate is green on Go 1.27.2: `go mod verify`, `go mod tidy -diff`, build, vet, `gofmt`,
  golangci-lint v2.14.0, `go test -race` with **0 races**; `govulncheck` reports **no
  vulnerabilities found**.

## v0.1.1

### Changed — if you scrape this service's metrics, read this before you deploy

The metrics endpoint no longer offers the OpenMetrics format. A scraper that asked for it by sending
`Accept: application/openmetrics-text` was answered in that format, with the `# EOF` terminator it
requires; it is now answered in the Prometheus text format regardless of what it asks for, and no
`# EOF` is written:

```http
GET /metrics
Accept: application/openmetrics-text

200 OK
Content-Type: text/plain; version=0.0.4; charset=utf-8      ← was: application/openmetrics-text; version=1.0.0; charset=utf-8
```

**The metric names, labels and values are all unchanged**, so Prometheus and anything else that
accepts the plain-text format keeps working with nothing to do. Two setups need a look: a scrape
configuration that *requires* the OpenMetrics content type, and any check that treats a missing
`# EOF` as a truncated scrape.

This arrives from the web framework, not from a change of ours. **The platform's other services
crossed it on 2026-09-09 — this one did not, and crosses it now**, because it was left a release
behind on the shared platform library.

### Changed — the shared libraries move to their current releases

`go-platform-kit` v1.11.1 → v1.11.3 and `go-sec-events` v1.2.1, with the web framework to v0.38.1
and its HTTP stack to v1.74.0. Nothing else changes for you: no endpoint, field, error or setting,
and this service's own behaviour is unchanged — the hash-chained evidence record is appended exactly
as before, and the frozen event envelope is untouched. The Postgres driver `pgx/v5` moves to v5.11.0
in the same pass.

### Fixed — a version tag points at the signed image digest again

Publishing a release re-pointed the version tag by rewrapping the image manifest into a new manifest
list, which gave the tag a **different digest from the one the signature covers** — so verifying the
signature on a version tag failed. The retag is now a plain pull, tag and push, which keeps the
digest and therefore keeps the signature valid. Separately, a release published right after a merge
could race the branch build; the job now waits for the image to be published before tagging.

**A version tag published before this needs one release re-publish** to be re-pointed at the signed
digest.

## v0.1.0

Initial code.

The signing-evidence sink as first released: the append-only, hash-chained legal record of who
applied which signature to which document, when, and at what assurance level. Producers freeze
each signing event into a broker envelope and publish it; the service verifies the chain and
appends the event tamper-evidently. MIT.
