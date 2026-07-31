---
title: Encrypt backups per tenant
status: superseded
supersedes:
  - 0001
date: 2025-03-17
---

# Encrypt backups per tenant

## Context and Problem Statement

A single shared key means one leaked key exposes every tenant's backup.

## Decision Outcome

Each tenant gets its own symmetric key. A backup is encrypted with the key of the
tenant it belongs to.

## Consequences

Blast radius shrinks to one tenant. Key count now grows with tenant count.
