---
title: Authenticate browsers with session cookies
status: superseded
date: 2025-01-13
---

# Authenticate browsers with session cookies

## Context and Problem Statement

The web client needs an authenticated session that survives a page reload without
storing anything reachable from scripts.

## Decision Outcome

A signed, http-only session cookie is issued at sign-in and validated against a
server-side session table on every request.

## Consequences

Revocation is immediate because the session row can be deleted. Every request
costs one session lookup.
