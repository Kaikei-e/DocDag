---
title: Store sessions in the database
status: superseded
date: 2025-01-06
---

# Store sessions in the database

Sessions lived in the same database as everything else, so a login cost a row
write and every request cost a read.
