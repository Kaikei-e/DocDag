---
title: Version the public API in the path
status: accepted
date: 2025-03-04
---

# Version the public API in the path

## Context and Problem Statement

Clients we do not control call the public API, so a breaking change needs a way
to coexist with the old contract.

## Decision Outcome

The major version lives in the path, as in /v1/articles. A new major version is a
new path prefix, served side by side with the previous one.

## Consequences

Versions are visible in logs, proxies and bug reports. Two contracts have to be
maintained for the length of the deprecation window.
