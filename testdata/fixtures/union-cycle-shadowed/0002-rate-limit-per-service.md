---
title: Rate limit per service
status: superseded
supersedes:
  - 0001
depends-on:
  - 0003
date: 2025-02-20
---

# Rate limit per service

## Context and Problem Statement

The gateway limit throttled cheap and expensive endpoints alike, so a service
that could take the load was held back by one that could not.

## Decision Drivers

- A limit that reflects what an endpoint costs
- No single component whose failure removes every limit
- Limits readable next to the code they protect

## Considered Options

- A limiter in each service
- Per-route limits in the gateway
- Keep the gateway limit

## Decision Outcome

Chosen option: a limiter in each service, sharing the counter store the
quota decision settled on.

## Consequences

Every service now carries limiter configuration, and the counter store became
a dependency of all of them.
