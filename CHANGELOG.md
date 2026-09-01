# Changelog

Notable changes to this service, newest first, per release. This file is written for whoever
runs the service or integrates against it.

## v0.1.0

Initial code.

The signing-evidence sink as first released: the append-only, hash-chained legal record of who
applied which signature to which document, when, and at what assurance level. Producers freeze
each signing event into a broker envelope and publish it; the service verifies the chain and
appends the event tamper-evidently. MIT.
