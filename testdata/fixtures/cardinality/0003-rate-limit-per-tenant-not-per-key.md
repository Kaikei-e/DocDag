---
title: Rate limit per tenant, not per key
status: accepted
supersedes:
  - 0001
date: 2025-03-25
---

# Rate limit per tenant, not per key

## Context and Problem Statement

Tenants issue as many API keys as they like, so a per-key limit multiplied by
the number of keys a tenant held. The limit a contract names is per tenant.

## Decision Drivers

- The enforced limit must be the one the contract names
- Issuing a key must not raise a tenant's throughput
- Per-key visibility must survive for debugging

## Considered Options

- Keep the per-key limit and cap the number of keys
- Move the limit to the tenant and report usage per key
- Enforce both limits at once

## Decision Outcome

Chosen option: the enforced bucket is per tenant, while usage is still reported
per key so an operator can see which key spent the allowance.

## Consequences

The enforced limit matches the contract. A noisy key inside a tenant can still
starve that tenant's other keys, which the usage report makes visible.
