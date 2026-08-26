---
title: Rate limit per API key
status: superseded
date: 2025-01-09
---

# Rate limit per API key

## Context and Problem Statement

One customer's batch job saturated the API for everyone else. Nothing bounded
how fast a single caller could go.

## Decision Drivers

- One caller must not degrade the service for the rest
- The limit must be visible to the caller before it is hit
- No shared state beyond what the gateway already keeps

## Considered Options

- A fixed window counter per API key
- A token bucket per API key
- A global concurrency cap

## Decision Outcome

Chosen option: a fixed window counter per API key in the gateway, returning
429 with a `Retry-After` header once the window is exhausted.

## Consequences

A single caller can no longer starve the rest. Bursts at a window boundary can
still deliver twice the nominal rate.
