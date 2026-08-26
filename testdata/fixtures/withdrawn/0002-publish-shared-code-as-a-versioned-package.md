---
title: Publish shared code as a versioned package
status: accepted
date: 2025-02-13
---

# Publish shared code as a versioned package

## Context and Problem Statement

Six services copied the same client, logging setup and error types, and a fix
in one copy reached the others whenever somebody noticed.

## Decision Drivers

- One fix must be able to reach every service
- A service must be able to adopt a fix on its own schedule
- No repository restructuring

## Considered Options

- Copy the code and reconcile by review
- Publish a versioned internal package
- Merge every service into one repository

## Decision Outcome

Chosen option: publish the shared code as a versioned internal package, with a
release-please style changelog so consumers can see what a bump contains.

## Consequences

A fix is published once. Every service now carries a dependency version, and a
stale one is visible in the dependency report.
