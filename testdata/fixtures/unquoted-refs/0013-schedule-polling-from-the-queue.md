---
title: Schedule polling from the queue
status: accepted
supersedes: [0011]
depends-on: [0009, 0011]
date: 2025-04-23
---

# Schedule polling from the queue

## Context and Problem Statement

Backoff held in the poller's memory reset on every deploy, and the whole fleet
then fetched every feed at once.

## Decision Outcome

The next fetch is a delayed message on the queue: its visibility deadline is the
schedule, so backoff survives a restart.

## Consequences

The poller is stateless. Rescheduling a feed means republishing a message rather
than mutating memory.
