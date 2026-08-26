---
title: Adopt a monorepo for every service
status: withdrawn
date: 2025-01-22
---

# Adopt a monorepo for every service

## Context and Problem Statement

Six services lived in six repositories, and a change that touched a shared
library meant six pull requests in a fixed order.

## Decision Drivers

- A cross-cutting change should be one review
- Tooling should not have to be installed six times
- Release cadence must stay per service

## Considered Options

- One repository holding every service
- A shared library published as a versioned package
- Keep separate repositories and script the ordering

## Decision Outcome

Withdrawn before it was decided: the team that raised it moved the shared code
into a versioned package instead, which removed the problem this decision was
written to solve. Nothing supersedes it, because nothing replaced it.

## Consequences

None. The proposal was never in force.
