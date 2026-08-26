---
title: Sign release artifacts
status: accepted
tags:
  - security
reviewers:
  - security-guild
  - platform-guild
date: 2025-03-04
---

# Sign release artifacts

## Context and Problem Statement

Release archives were published from a build job that anyone with write access
to the repository could edit, and nothing downstream could tell a genuine
archive from a replaced one.

## Decision Drivers

- A consumer must be able to verify what they downloaded
- The signing key must not live in the repository
- Verification must not need a private service

## Considered Options

- Publish a checksum file next to the archives
- Sign with a long-lived key held in the secret store
- Sign with short-lived keys from the CI provider's identity

## Decision Outcome

Chosen option: sign with short-lived keys derived from the CI provider's
workload identity, publishing both the signature and the checksum file.

## Consequences

A consumer can verify a release without trusting the publishing job. Releases
now depend on the CI provider's identity service being available.
