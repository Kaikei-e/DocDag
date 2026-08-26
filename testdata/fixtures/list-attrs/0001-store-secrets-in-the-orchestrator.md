---
title: Store secrets in the orchestrator
status: accepted
tags:
  - security
  - platform
reviewers:
  - platform-guild
date: 2025-01-28
---

# Store secrets in the orchestrator

## Context and Problem Statement

Credentials lived in environment files checked into a private repository, so
every engineer with repository access held every production credential.

## Decision Drivers

- A credential must not be readable by everyone with repository access
- Rotation must not require a code change
- The delivery path must work for every workload we run

## Considered Options

- Keep environment files and restrict the repository
- Use the orchestrator's secret objects
- Run a dedicated secret manager

## Decision Outcome

Chosen option: the orchestrator's secret objects, mounted as files, with access
governed by the same roles that govern deployments.

## Consequences

Credentials are no longer in version control. The orchestrator's access control
is now the boundary that protects them, so its roles need review.
