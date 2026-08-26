---
title: Issue per-service identities from a CA
status: accepted
supersedes:
  - 0001
date: 2025-03-20
---

# Issue per-service identities from a CA

## Context and Problem Statement

The shared bearer token gave every service the authority of every other, and an
audit asked how a compromised service would be contained.

## Decision Drivers

- A compromised service must not be able to act as another
- Credentials must rotate without a fleet restart
- The change must be incremental, service by service

## Considered Options

- Per-service bearer tokens issued by hand
- Short-lived certificates from an internal CA
- A service mesh with its own identity plane

## Decision Outcome

Chosen option: short-lived certificates from an internal CA, delivered by the
workload orchestrator and renewed well before expiry.

## Consequences

Identity is now per service and rotation is automatic. The CA is a new
component whose availability every deploy depends on.
