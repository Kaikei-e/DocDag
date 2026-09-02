---
title: Store sessions in Redis
status: accepted
supersedes:
  - 0001
date: 2025-03-11
---

# Store sessions in Redis

The session store is the one thing read on every request, so it moved to a
store that expires keys by itself.
