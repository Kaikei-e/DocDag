---
title: Rate limit with a token bucket
status: accepted
supersedes:
  - 0001
date: 2025-02-17
---

# Rate limit with a token bucket

## Context and Problem Statement

The fixed window let a caller spend its whole allowance in the last second of
one window and again in the first second of the next, doubling the real rate.

## Decision Drivers

- The enforced rate must match the published rate
- A short burst must stay allowed
- The gateway must not need a new datastore

## Considered Options

- Shorten the window
- A sliding window log
- A token bucket with a burst size

## Decision Outcome

Chosen option: a token bucket per API key with a burst size of twice the
per-second rate, held in the gateway's existing key-value store.

## Consequences

The enforced rate now matches the published one. The bucket state is one more
key per caller, expiring after an idle period.
