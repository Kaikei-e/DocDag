---
title: Rate limit in the gateway
status: superseded
supersedes:
  - 0002
date: 2025-01-14
---

# Rate limit in the gateway

## Context and Problem Statement

A single noisy client could exhaust the request pool, and the gateway was the
only component every request already passed through.

## Decision Drivers

- One place to configure a limit
- No per-service bookkeeping
- The limit must survive a service restart

## Considered Options

- A token bucket in the gateway
- A per-service limiter
- No limit at all

## Decision Outcome

Chosen option: a token bucket in the gateway, keyed by API key.

## Consequences

The gateway became the place a limit is changed, and the place an outage
starts. That is the trade this pair of decisions kept swapping between.
