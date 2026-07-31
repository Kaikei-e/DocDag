---
title: Ship logs to a central collector
status: accepted
supersedes:
  - 0009
date: 2025-02-11
---

# Ship logs to a central collector

## Context and Problem Statement

Logs sat on each host, so answering any cross-service question meant opening
several shells.

## Decision Outcome

A sidecar tails each container's stdout and ships lines to a central collector.
Hosts keep a short local buffer so a collector outage loses nothing.

## Consequences

Cross-service queries become possible. The collector is now a dependency that
needs its own capacity planning.
