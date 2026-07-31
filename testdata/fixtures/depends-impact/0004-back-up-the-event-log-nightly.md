---
title: Back up the event log nightly
status: accepted
depends-on:
  - 0001
date: 2025-03-05
---

# Back up the event log nightly

## Context and Problem Statement

Read models can be rebuilt, so the event log is the only data whose loss would be
unrecoverable.

## Decision Outcome

The event log is backed up every night and the restore is exercised once a month.
Read models are excluded from backups entirely.

## Consequences

Backup volume stays small and the restore path is the one we actually practise.
A restore is followed by a full reprojection, which takes time.
