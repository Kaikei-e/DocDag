---
title: Store the poll cursor per feed
status: accepted
date: 2025-04-09
---

# Store the poll cursor per feed

## Context and Problem Statement

A poller that keeps its position in memory refetches everything after a restart,
and two pollers of one feed disagree about what has been seen.

## Decision Outcome

The cursor is a row beside the feed, written when a fetch succeeds.

## Consequences

A restart resumes where the last fetch ended. The cursor is now a schema object
with the migration cost that implies.
