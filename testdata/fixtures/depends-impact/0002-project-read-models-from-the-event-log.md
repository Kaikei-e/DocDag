---
title: Project read models from the event log
status: accepted
depends-on:
  - 0001
date: 2025-01-22
---

# Project read models from the event log

## Context and Problem Statement

Queries against the append-only table of [[0001]] have to fold the whole history
of an entity, which is far too slow for a list view.

## Decision Outcome

Projectors consume the event log and maintain read models. A read model is
disposable: it can be dropped and rebuilt from the log at any time.

## Consequences

Reads are fast and the schema of a read model can change freely. Projections lag
the log by the projector's processing delay.
