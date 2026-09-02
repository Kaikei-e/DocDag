---
title: Move poison jobs to a dead-letter queue
status: accepted
amends:
  - 0002
depends-on:
  - 0005
date: 2025-03-18
---

# Move poison jobs to a dead-letter queue

The amendment names the current queue, so the first constraint holds. The
second does not: the decision it amends depends on 0006, and this one does not,
so amending it drags in a dependency nobody declared here.
