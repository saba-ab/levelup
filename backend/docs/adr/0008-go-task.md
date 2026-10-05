# ADR-0008: go-task over GNU Make

**Status:** Accepted · 2026-09-01

## Context
Nothing in this repo is a C-style file-dependency graph; Make's tab syntax, recursive
expansion and `.PHONY` bookkeeping are pure tax (PRD §15).

## Decision
`Taskfile.yml` with typed vars, `requires` for mandatory args, `preconditions` with
readable messages, `sources/generates` for incremental skipping, dotenv loading.
CI invokes the same targets (`task check`) — one definition of every command.

## Consequences
One more binary to install (`brew install go-task`). Divergence between local and CI
commands is structurally impossible.
