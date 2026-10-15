# ADR 0016: Go for the data pipeline, Python for the agents

- **Status:** Accepted, implemented: Go pipeline, and the Python orchestrator in `agents/` ([ADR 0022](0022-orchestrator-asyncio-pipeline.md))
- **Date:** 2026-10-15

## Context

Go is the author's strongest language and suits a fast, dependency-free streaming core. Microsoft Agent Framework and Foundry have their most complete support and samples in Python and .NET.

## Decision

Simulator, stats engine, moment rules, store and MCP server are in Go. The agent orchestrator and agents will be in Python with Microsoft Agent Framework, hosted on Microsoft Foundry. The boundary is the MCP tools, the JSON Schemas and the JSON API.

## Consequences

- Each side uses the strongest tooling for its job.
- Two languages to set up, test and deploy. CI will gain a Python job.
- The contracts in [ADR 0007](0007-contracts-go-types-and-json-schemas.md) keep the two sides honest.
