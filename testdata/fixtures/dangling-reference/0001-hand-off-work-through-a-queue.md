---
title: Hand off work through a queue
status: accepted
date: 2025-04-02
---

# Hand off work through a queue

## Context and Problem Statement

Thumbnail rendering ran inside the request that asked for it, so a slow render
held a connection open and a burst of uploads exhausted the worker pool.

## Decision Drivers

- A slow render must not occupy a request worker
- Work must survive a process restart
- No new operational surface beyond the database we already run

## Considered Options

- Render inline and cap the concurrency
- Hand work to a durable queue and render in a worker
- Render ahead of time on upload

## Decision Outcome

Chosen option: hand work to a durable queue backed by the existing database and
render in a separate worker pool.

## Consequences

Request latency no longer depends on render time. The queue is now a component
with its own depth and age to watch.
