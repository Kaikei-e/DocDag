---
title: Pin the CA bundle in every image
status: accepted
superseded_by:
  - 0002
date: 2025-04-08
---

# Pin the CA bundle in every image

## Context and Problem Statement

Services verify each other against the internal CA, so every image needs the CA
bundle. Reading it from the host meant a host upgrade could break verification.

## Decision Drivers

- Verification must not depend on the host filesystem
- A CA rotation must be a deliberate, reviewable change
- Images must stay reproducible

## Considered Options

- Mount the host bundle into every container
- Bake the bundle into the base image
- Fetch the bundle from the CA at startup

## Decision Outcome

Chosen option: bake the bundle into the base image, so a rotation is a base
image bump that goes through review like any other change.

## Consequences

Verification no longer depends on the host. A CA rotation now requires
rebuilding and redeploying every service, which is slow but visible.
