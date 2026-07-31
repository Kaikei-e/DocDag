---
title: Run migrations during deploy
status: superseded
date: 2025-02-06
---

# Run migrations during deploy

## Context and Problem Statement

Schema changes have to be applied before the code that depends on them starts
serving traffic.

## Decision Outcome

The deploy pipeline runs pending migrations as a step before the rollout, and
aborts the rollout if any migration fails.

## Consequences

Schema and code move together. A long migration blocks the deploy, and a failed
migration halfway through leaves the schema in an unclear state.
