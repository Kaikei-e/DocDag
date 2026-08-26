---
title: Size the connection pool per instance
status: accepted
depends-on:
  - 0003
date: 2025-03-13
---

# Size the connection pool per instance

## Context and Problem Statement

Every instance opened as many connections as it liked, and the database hit its
connection limit long before it hit its CPU limit.

## Decision Drivers

- The fleet must not exceed the server's connection limit
- A single slow query must not starve the rest of an instance
- The number must be derivable, not guessed

## Considered Options

- A fixed pool size per instance
- A pool sized from the server limit divided by the instance count
- A shared connection proxy in front of the database

## Decision Outcome

Chosen option: derive the per-instance size from the server limit and the
maximum instance count set by the autoscaling decision, leaving headroom for
administrative sessions.

## Consequences

The fleet can no longer exhaust the server's connections. Raising the instance
ceiling now requires revisiting the pool size.
