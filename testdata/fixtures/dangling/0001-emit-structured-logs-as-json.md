---
title: Emit structured logs as JSON
status: accepted
date: 2025-01-30
---

# Emit structured logs as JSON

## Context and Problem Statement

Free-form log lines are cheap to write and expensive to query. Every incident
review ended with someone writing a one-off regular expression.

## Decision Outcome

Every service emits one JSON object per line, with a fixed set of required keys:
timestamp, level, service, message and request id.

## Consequences

Logs are queryable without parsing rules. Lines are larger, and human readers
need a formatter to read them comfortably.
