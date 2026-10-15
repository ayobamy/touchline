# ADR 0001: Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-10-15

## Context

Touchline is built by one person in 13 days for a judged hackathon. Judges score "quality software development practices" and "well-structured, documented, and maintainable code". Choices made quickly under time pressure are easy to forget, and easy to second-guess later without the reasoning at hand.

## Decision

Record each significant decision as a short Architecture Decision Record in `docs/adr/`, numbered in order, using this template:

- **Status:** Proposed, Accepted, or Superseded by ADR NNNN. Accepted records also say whether the decision is implemented yet.
- **Context:** the forces at play.
- **Decision:** what we chose.
- **Consequences:** what gets easier, what gets harder.
- **Alternatives considered.**
- **In the code:** where the decision lives, once implemented.

A record is never rewritten to reverse it. A new record supersedes it, and the old one's status is updated to point at the new one.

## Consequences

- Reasoning survives the hackathon and is visible to judges.
- Each new decision costs a few minutes to write down.
