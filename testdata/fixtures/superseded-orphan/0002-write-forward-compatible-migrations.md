---
title: Write forward-compatible migrations
status: accepted
date: 2025-03-19
---

# Write forward-compatible migrations

## Context and Problem Statement

During a rollout, the old and the new code run against the same schema. A
migration that only suits the new code breaks the old one.

## Decision Outcome

Every migration must be readable by the currently deployed code: columns are
added before they are written, and dropped only one release after last use.

## Consequences

Rollout and rollback both stay safe. A rename becomes a sequence of releases
rather than a single change.
