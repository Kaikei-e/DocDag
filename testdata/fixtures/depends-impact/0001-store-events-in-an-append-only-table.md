---
title: Store events in an append-only table
status: accepted
date: 2025-01-08
---

# Store events in an append-only table

## Context and Problem Statement

State was kept as mutable rows, so the question "how did this record get into
this state" had no answer after the fact.

## Decision Outcome

Every state change is written as an event row. The table takes inserts only:
there is no update and no delete path.

## Consequences

History becomes queryable and reproducible. Reading current state now requires a
projection rather than a single row.
