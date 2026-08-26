---
title: Write audit events to the database
status: superseded
date: 2025-02-04
---

# Write audit events to the database

## Context and Problem Statement

Compliance asked for a record of every privileged action. The application
already owns a transactional database, and nothing else was in place.

## Decision Drivers

- An audit record must be written in the same transaction as the action
- The record must be readable by an auditor without a new tool
- Volume is a few thousand rows a day

## Considered Options

- A table in the application database
- A log line with a reserved prefix
- A dedicated audit service

## Decision Outcome

Chosen option: an `audit_events` table written inside the transaction that
performs the action, so a rolled-back action leaves no audit record behind.

## Consequences

Audit and application data share a backup and a retention policy. The table
grows without bound until something prunes it.
