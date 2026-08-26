---
title: Share one counter store
status: accepted
depends-on:
  - 0001
date: 2025-03-05
---

# Share one counter store

## Context and Problem Statement

Per-service limiters each kept their own counters, so a client hitting three
services got three times its quota.

## Decision Drivers

- One quota per client, wherever the request lands
- A counter read must not cost more than the work it guards
- No new operational component if one already fits

## Considered Options

- A shared counter store
- Sticky routing per client
- Accept the multiplied quota

## Decision Outcome

Chosen option: a shared counter store, reached through the gateway's own
connection so the limiting decision keeps one owner.

## Consequences

The counter store is now on the request path of every limited endpoint, and
the gateway decision it leans on is the one that started this chain.
