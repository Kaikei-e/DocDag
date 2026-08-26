---
title: Stream audit events to append-only storage
status: accepted
supersedes:
depends-on: []
date: 2025-06-30
---

# Stream audit events to append-only storage

## Context and Problem Statement

The audit table shares a database with the application, so an operator with
write access to one has write access to the other. An auditor asked for a store
the application cannot rewrite.

## Decision Drivers

- The application must not be able to alter a written record
- Retention must be enforced by the store, not by a cron job
- The write must still fail the action when it fails

## Considered Options

- Keep the table and revoke UPDATE and DELETE from the application role
- Stream events to append-only object storage with an immutability policy
- Send events to a managed audit service

## Decision Outcome

Chosen option: stream every event to append-only object storage with a
retention lock, keeping a short-lived table as the outbox that guarantees the
write happens in the action's transaction.

## Consequences

The application can no longer rewrite history. An auditor reads the bucket
directly, and the outbox is one more thing that can fall behind.
