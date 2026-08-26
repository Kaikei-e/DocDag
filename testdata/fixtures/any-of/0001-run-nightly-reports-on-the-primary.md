---
title: Run nightly reports on the primary database
status: deprecated
date: 2025-01-16
---

# Run nightly reports on the primary database

## Context and Problem Statement

Finance needed a nightly export, and the primary database was the only place
the data lived in one consistent shape.

## Decision Drivers

- The export must see a consistent snapshot
- Nothing new to operate before the first export
- The window between midnight and 04:00 is quiet

## Considered Options

- Query the primary inside the quiet window
- Restore last night's backup and query that
- Stand up a read replica

## Decision Outcome

Chosen option: run the export against the primary inside the quiet window,
inside a repeatable-read transaction.

## Consequences

The export sees a consistent snapshot with no new components. A long export
holds a transaction open, which delays vacuuming until it finishes.
