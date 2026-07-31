---
title: Choose a log retention window
status: accepted
date: 2025-01-20
---

# Choose a log retention window

## Context and Problem Statement

Retention is a trade between storage cost and how far back an investigation can
reach. We never wrote the trade down, so every team assumed a different answer.

## Decision Outcome

Retention is decided per log class rather than globally. Each class records its
own window in a follow-up decision.

## Consequences

Cost becomes attributable per class. Someone has to keep the class list current.
