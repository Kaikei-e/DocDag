---
title: Authenticate integrations with API keys
status: superseded
date: 2025-01-28
---

# Authenticate integrations with API keys

## Context and Problem Statement

Server-to-server integrations have no browser, so cookies do not apply, and they
need credentials that can be rotated without a human signing in.

## Decision Outcome

Each integration gets a long-lived API key sent in an authorization header. Keys
are stored hashed and can be revoked individually.

## Consequences

Integrations are simple to onboard. A leaked key stays valid until somebody
notices and revokes it.
