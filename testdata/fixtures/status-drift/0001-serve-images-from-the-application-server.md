---
title: Serve images from the application server
status: accepted
date: 2025-01-27
---

# Serve images from the application server

## Context and Problem Statement

Images have to be served from somewhere, and the application server already
handles every other request.

## Decision Outcome

The application server serves images directly from its own storage, with a long
cache lifetime on the response.

## Consequences

Nothing new to deploy. Image traffic competes with API traffic for the same
connection pool and the same CPU.
