---
title: Retry failed jobs with exponential backoff
status: accepted
date: 2025-05-19
---

# Retry failed jobs with exponential backoff

## Context and Problem Statement

The queue introduced in [[0001]] drops a job the first time its handler raises.
Most failures are a downstream service refusing a connection for a few seconds,
so a single attempt throws away work that would have succeeded on the next one.

## Decision Drivers

- A transient downstream failure must not lose work
- A permanently broken job must not retry forever
- Operators must be able to see why a job stopped

## Considered Options

- Retry a fixed number of times at a fixed interval
- Retry with exponential backoff and a dead-letter queue
- Push retries back onto the caller

## Decision Outcome

Chosen option: exponential backoff with full jitter, six attempts, then the job
moves to a dead-letter queue for inspection. The retention window agreed in
[[0009]] applies to the dead-letter queue as well.

The handler declares its own policy in frontmatter:

```yaml
depends-on:
  - [[0042]]
```

## Consequences

Transient failures cost latency instead of work. The dead-letter queue is a new
place an operator has to look, and `retry_count` is now part of every job row.
