---
title: Cache rendered thumbnails
status: accepted
date: 2025-01-09
---

# Cache rendered thumbnails

## Context and Problem Statement

Rendering a thumbnail costs about 40 ms of CPU. The same image is requested by
every reader who opens the same article, so the work is repeated for no gain.

## Decision Drivers

- Render cost dominates the article list endpoint
- Thumbnails never change once rendered
- Cache correctness must not depend on invalidation

## Considered Options

- Render on every request
- Cache rendered thumbnails keyed by source URL and size
- Pre-render every thumbnail at ingestion time

## Decision Outcome

Chosen option: cache rendered thumbnails keyed by source URL and requested size.
The key is content-addressed, so an entry is never stale and never invalidated.

## Consequences

The list endpoint stops burning CPU on repeat renders. Where the cache lives is
left open here and settled separately.
