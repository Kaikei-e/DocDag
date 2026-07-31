---
title: Expire thumbnails after thirty days
status: proposed
depends-on:
  - 0003
date: 2025-04-16
---

# Expire thumbnails after thirty days

## Context and Problem Statement

The bucket grows without bound. Most thumbnails are read during the week after
ingestion and never again.

## Decision Drivers

- Storage cost grows linearly and forever
- A miss is cheap: the thumbnail can be rendered again
- Expiry must not need a bespoke sweeper process

## Considered Options

- Keep everything
- A lifecycle rule that expires objects thirty days after their last write
- A least-recently-used sweeper we build and run ourselves

## Decision Outcome

Proposed: a lifecycle rule on the bucket, so expiry is configuration rather than
code.

## Consequences

Storage stabilises at roughly one month of traffic. An article resurfacing after
a long silence pays one render.
