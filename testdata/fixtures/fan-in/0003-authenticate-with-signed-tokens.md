---
title: Authenticate every client with short-lived signed tokens
status: accepted
supersedes:
  - 0001
  - 0002
date: 2025-05-20
---

# Authenticate every client with short-lived signed tokens

## Context and Problem Statement

Two authentication paths meant two implementations of every authorization rule,
and the rules had already drifted apart in three places.

## Decision Outcome

Browsers and integrations both exchange their credential for a short-lived signed
token. The token carries the subject and the scopes, and services verify it with
a shared public key.

## Consequences

One verification path serves every client. Revocation is no longer immediate: it
takes effect when the current token expires.
