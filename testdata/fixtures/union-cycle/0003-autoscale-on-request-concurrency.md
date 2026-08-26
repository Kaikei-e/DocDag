---
title: Autoscale on request concurrency
status: accepted
supersedes:
  - 0001
date: 2025-04-21
---

# Autoscale on request concurrency

## Context and Problem Statement

Scaling on CPU added instances long after latency had already risen, because
the bottleneck was waiting on the database rather than burning CPU.

## Decision Drivers

- The signal must move before latency does
- The ceiling must be a number the database can sustain
- Scale-in must not sever in-flight requests

## Considered Options

- Scale on CPU utilisation
- Scale on in-flight request concurrency
- Scale on queue depth

## Decision Outcome

Chosen option: scale on in-flight concurrency per instance, with a hard ceiling
set from what the database can serve, which also replaces the database-backed
session decision with signed tokens.

## Consequences

The fleet reacts before latency rises. The ceiling ties the fleet size to the
database, so the two now move together.
