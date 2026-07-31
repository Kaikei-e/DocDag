---
title: Encrypt backups with a shared key
status: superseded
supersedes:
  - 0003
date: 2025-02-05
---

# Encrypt backups with a shared key

## Context and Problem Statement

Backups leave our network and must be unreadable to whoever stores them.

## Decision Outcome

Every backup is encrypted with one symmetric key held in the deployment secret
store.

## Consequences

Simple to implement. Rotating the key means rewriting every retained backup.
