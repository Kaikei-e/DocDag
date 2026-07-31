---
title: Serve images from a CDN
status: accepted
supersedes:
  - 0001
date: 2025-04-14
---

# Serve images from a CDN

## Context and Problem Statement

Image requests outnumber API requests by twenty to one and saturated the
application server's connection pool during peak hours.

## Decision Outcome

Images are served by a CDN with the application server as origin. The origin is
only ever reached on a cache miss.

## Consequences

The application server sees a fraction of the previous traffic. Purging is now an
explicit operation rather than an implicit one.
