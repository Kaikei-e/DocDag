---
title: Keep sessions in the database
status: superseded
depends-on:
  - 0002
date: 2025-02-06
---

# Keep sessions in the database

## Context and Problem Statement

Sessions had to survive a deploy, and the database was the only durable store
the application already talked to.

## Decision Drivers

- A deploy must not sign everyone out
- No new component to operate
- Session lookup is on every request, so it must be fast

## Considered Options

- A sessions table in the application database
- Signed cookies with no server state
- A dedicated session store

## Decision Outcome

Chosen option: a sessions table, read through the connection pool sizing agreed
in the pooling decision.

## Consequences

Sessions survive deploys. Every request now costs a database round trip, which
is what pushed the pool sizing question in the first place.
