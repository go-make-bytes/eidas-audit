# Security policy

This service is the **evidence trail for legally effective signatures** — the append-only,
hash-chained record of who applied which signature to which document, when, and at what assurance
level. Nothing here signs anything and nothing here decides anything: signing services freeze an
event into an envelope and publish it to a broker, and this service is the durable consumer that
verifies the chain link and appends it, permanently.

Evidence is only worth what its integrity is worth, so the failures that matter most are the ones
that make the record **untrue** rather than the ones that make it unavailable. Two shapes:

- **A record that can be changed, inserted or removed without the chain saying so.** The whole
  construction — link each event to its predecessor over canonical bytes, refuse a break rather
  than absorb it — exists so that an after-the-fact edit is detectable by re-walking the chain.
- **A signing event that never lands, quietly.** A missing link in an evidence trail is not a
  degraded service, it is a signature nobody can later prove was applied. Delivery is at-least-once
  and a failed append is redelivered on purpose.

Please report security problems privately. Do not open a public issue, pull request or discussion
for anything that could be exploited before a fix exists.

## How to report

Use **[private vulnerability reporting](https://github.com/go-make-bytes/eidas-audit/security/advisories/new)**
on this repository. The report stays visible only to you and the maintainers until an advisory is
published, and it gives us one place to discuss and co-ordinate a fix with you.

Please include, as far as you can establish it:

- what the problem is, and what an attacker gains from it;
- the smallest set of steps that reproduces it, and against which version or commit;
- the configuration it needs, if it only appears under particular settings;
- whether you have told anyone else, and whether a disclosure date already binds you.

**Please do not send us real signing-event envelopes, broker credentials or national identifiers.**
A redacted example, or the shape of the value, explains almost any finding here.

## What happens next

- We acknowledge a report within **five working days**.
- We tell you whether we can reproduce it, and what we think its severity is, as soon as we know.
- We keep you updated while a fix is prepared, and we agree a disclosure date with you. Our default
  is to publish an advisory once a fix is available, and in any case within **90 days** of the
  report — earlier if the problem is already public or being exploited.
- We credit you in the advisory unless you would rather stay anonymous.

There is no bug-bounty programme. We are grateful anyway, and we say so publicly.

## What we consider most serious

**Anything that breaks the tamper-evidence of the chain.**

- A stored event that can be altered or deleted so that a re-walk still verifies. That is the single
  property the whole design buys, and any route to it — through the store, through a procedure,
  through a replay — is the most serious class of finding here.
- An append that is accepted when the link does **not** continue from the current head. A break is
  meant to be rejected and retried, never absorbed, never repaired by rewriting history.
- Two different envelopes that hash to the same canonical bytes, or a stored envelope that can be
  re-serialised into a different one that still matches its link. The chain is only as strong as the
  canonicalisation under it.
- A path that re-computes the chain from current rows rather than verifying against what was
  written. Recomputation cannot detect the edit it is supposed to detect.

**Anything that loses an event, or lands a false one.**

- An event acknowledged to the broker without being durably appended. At-least-once delivery is safe
  only because a failed append is negatively acknowledged and redelivered; an acknowledgement on a
  failure path silently truncates the evidence trail.
- Duplicate suppression that drops a *distinct* event, or idempotency on `event_id` that lets one
  event overwrite another.
- An event reaching the store from anything other than the trusted producer path — a forged or
  replayed broker message that is appended as if a signature had happened.

**What must never be in this store, or leak out of it.**

- Document content, free text, or any personal data beyond the pseudonymous references the envelope
  contract allows. This is a permanent, court-grade record: anything that lands in it is very hard
  to unland, which is exactly why the producer strips token-shaped values and the sink stores
  references rather than identities.
- Broker credentials or TLS key material reaching a log line, an error body or a stored record.
- A national identifier appearing unpseudonymised anywhere — a log, an event, an error.

**Configuration that degrades the durable path silently.** Boot is deliberately loud about the
development fallbacks: no broker means the consumer does not start, and no store DSN means an
in-memory chain that dies with the process. A configuration that reaches production in either state
without saying so is a real finding, because the service *looks* healthy while recording nothing.

Denial of service and findings that need an already-compromised host or an already-authenticated
administrator are in scope but lower priority. Reports about outdated dependencies are welcome
where you can show the vulnerable path is actually reachable.

## What is deliberately not a finding

This service **records; it does not sign and it does not judge**. It applies no signature, evaluates
no certificate, and makes no trust decision about the event it is given — those belong to the
signing and validation services upstream. That an event described a signature inaccurately is a
finding against its producer, not against this service, unless this service mishandled what it was
given. A report that an API *implies* this service validated the signature it recorded **is** a real
finding.

Two documented limitations are not vulnerabilities in themselves, though a concrete exploitation of
either is: there is no read/verify HTTP API yet, so chain verification is done out of band; and the
canonical form depends on a lossless envelope round-trip rather than a format-independent
canonicalisation.

## Scope

This policy covers the code in this repository. It does not cover the emitter library the producers
use, the broker, the database schema and its procedures as deployed by an operator, or any
deployment operated by someone other than us — report those to the parties that run them. How a
deployment configures this service is the operator's responsibility, but a report that a **default**
is unsafe is very much in scope.

## Supported versions

Security fixes land on the most recent release. Older tags are not patched; if you are pinned to
one, the fix is to move forward.
