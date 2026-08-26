---
title: Query reports from a read replica
status: accepted
date: 2025-03-18
---

# Query reports from a read replica

## Context and Problem Statement

The nightly export needed a consistent snapshot without holding a transaction
open on the primary for the length of the export.

## Decision Drivers

- No long-running transaction on the primary
- The snapshot must still be consistent
- The team already operates replicas for failover

## Considered Options

- Shorten the export into several transactions
- Query a dedicated read replica
- Move reporting to a warehouse

## Decision Outcome

Chosen option: a dedicated read replica with a generous conflict window, so a
long report delays replay on that replica and nothing else.

## Consequences

The primary is untouched by reporting. The replica can fall behind during a
long report, which is acceptable because nothing fails over to it.
