---
title: Store thumbnails in object storage
status: accepted
depends-on:
  - 0001
date: 2025-03-11
---

# Store thumbnails in object storage

## Context and Problem Statement

Running a second instance made the local disk cache useless: each instance saw
roughly half the hits and re-rendered the rest.

## Decision Drivers

- The cache must be shared by every instance
- A replaced instance must not cost us a cold cache
- Read latency may grow, render cost may not

## Considered Options

- Replicate the local disk cache between instances
- Put the cache in object storage behind a small read-through layer
- Move rendering into a dedicated service with its own cache

## Decision Outcome

Chosen option: object storage behind a read-through layer, keeping the
content-addressed key from the caching decision unchanged.

## Consequences

The hit rate is now independent of instance count. Reads cost a network round
trip, mitigated by a small in-process layer in front of the bucket.
