---
title: Encrypt backups with envelope keys
status: superseded
supersedes:
  - 0002
date: 2025-05-06
---

# Encrypt backups with envelope keys

## Context and Problem Statement

Per-tenant keys are still long-lived, and rotating one still means rewriting that
tenant's retained backups.

## Decision Outcome

Each backup is encrypted with a fresh data key, and that data key is wrapped by
the tenant key. Rotation rewraps data keys instead of rewriting backups.

## Consequences

Rotation becomes cheap. The wrapping service is now on the restore path and must
be available before any restore can start.
