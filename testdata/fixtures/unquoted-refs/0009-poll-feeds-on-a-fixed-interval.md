---
title: Poll feeds on a fixed interval
status: accepted
date: 2025-04-02
---

# Poll feeds on a fixed interval

## Context and Problem Statement

The reader had no schedule of its own: a feed was fetched whenever somebody
opened it, so a popular feed was hit constantly and a quiet one went stale.

## Decision Outcome

Every feed is polled on one interval, held by the scheduler rather than by the
request path.

## Consequences

Load is predictable and independent of who is reading. A feed that publishes
faster than the interval is read late.
