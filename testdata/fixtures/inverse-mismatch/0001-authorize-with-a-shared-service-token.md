---
title: Authorize with a shared service token
status: superseded
date: 2025-01-14
---

# Authorize with a shared service token

## Context and Problem Statement

Internal services had to prove who they were to each other, and the first
deadline was a week away.

## Decision Drivers

- Ship before the launch
- No new component to operate
- Revocable without a deploy

## Considered Options

- One shared bearer token distributed by configuration
- Per-service tokens issued by hand
- Mutual TLS between services

## Decision Outcome

Chosen option: a single shared bearer token, kept in the secret store and read
at startup, on the understanding that per-service identity follows later.

## Consequences

Every service can impersonate every other. Rotating the token restarts the
whole fleet, so rotation happens rarely, which is the opposite of the intent.
