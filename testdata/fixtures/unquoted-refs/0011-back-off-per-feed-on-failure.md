---
title: Back off per feed on failure
status: superseded
depends-on: [0010]
date: 2025-04-16
---

# Back off per feed on failure

## Context and Problem Statement

A feed that answers with an error is retried on the next tick like every other,
so an outage at one publisher costs as many requests as a healthy feed does.

## Decision Outcome

A failing feed doubles its own interval, up to a day, and resets on the first
success. The multiplier lives beside the cursor of
[[0010]].

## Consequences

An outage costs progressively fewer requests. A feed that recovers quietly is
read late for one interval.
