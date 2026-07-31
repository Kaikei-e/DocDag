---
title: Serve the dashboard from read models
status: accepted
depends-on:
  - 0002
date: 2025-02-17
---

# Serve the dashboard from read models

## Context and Problem Statement

The dashboard issues a dozen aggregate queries per page load, and running them
against the event log made the page unusable.

## Decision Outcome

Every dashboard panel reads exactly one read model built by the projectors from
[[0002]]. No dashboard query touches the event log directly.

## Consequences

Page load becomes a set of key lookups. A new panel needs a new projection rather
than a new query.
