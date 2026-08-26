---
title: Serve static assets from the application
status: accepted
tags:
  - legacy
reviewers:
  - platform-guild
date: 2025-02-11
---

# Serve static assets from the application

## Context and Problem Statement

The application had one deployment unit and a handful of images, and putting a
CDN in front of it was more machinery than the traffic justified.

## Decision Drivers

- The smallest number of moving parts
- Assets must be versioned with the code that uses them
- Traffic is low enough that a cache gains little

## Considered Options

- Serve assets from the application
- Publish assets to object storage behind a CDN
- Bundle assets into the client

## Decision Outcome

Chosen option: serve assets from the application with a long cache header and a
content hash in every asset path.

## Consequences

One deployment unit keeps assets and code in step. Asset traffic now competes
with API traffic for the same request workers.
