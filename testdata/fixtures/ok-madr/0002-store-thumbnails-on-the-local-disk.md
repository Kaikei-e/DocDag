---
title: Store thumbnails on the local disk
status: superseded by 0003
date: 2025-01-23
---

# Store thumbnails on the local disk

## Context and Problem Statement

The cache agreed in [the thumbnail cache decision](0001-cache-rendered-thumbnails.md)
needs somewhere to live. The service runs as a single instance today.

## Decision Drivers

- No new infrastructure
- Reads must stay below one millisecond
- Simple to operate for a single instance

## Considered Options

- A directory on the local disk
- An object store bucket
- A shared network filesystem

## Decision Outcome

Chosen option: a directory on the local disk, sharded by the first two characters
of the cache key.

## Consequences

Nothing new to operate, but the cache is bound to one machine and is lost on
every replacement of that machine.
