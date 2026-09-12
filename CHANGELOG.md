# Changelog

Notable changes to this service, newest first, per release. This file is written for whoever
runs the service or integrates against it.

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
