---
title: `X-Api-Version` selects the contract
status: proposed
date: 2025-04-08
---

# Negotiate API versions by header

## Context and Problem Statement

Path versioning duplicates every route. A header would let one route serve two
contracts.

## Decision Outcome

Proposed: clients send a version header and the router picks the contract. The
frontmatter of this document is deliberately invalid YAML.

## Consequences

Routes stop duplicating. Caches and proxies now have to vary on the header.
