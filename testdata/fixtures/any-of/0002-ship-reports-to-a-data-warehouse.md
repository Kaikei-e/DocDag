---
title: Ship reports to a data warehouse
status: rejected
tags:
  - explained
date: 2025-02-25
---

# Ship reports to a data warehouse

## Context and Problem Statement

The nightly export held a transaction open on the primary, and the obvious fix
was to move reporting somewhere else entirely.

## Decision Drivers

- Reporting must not touch the primary
- The cost must be proportional to the two reports we actually run
- The data must stay inside our own account

## Considered Options

- A managed data warehouse fed by change data capture
- A read replica queried directly
- Keep the export and shorten the transaction

## Decision Outcome

Rejected: a warehouse is the right answer for a reporting programme, and we
have two reports. The pipeline would cost more to run and to operate than the
reports are worth, so the replica option was taken instead.

## Consequences

Reporting stays on database infrastructure the team already operates. If the
number of reports grows, this decision is worth reopening.
