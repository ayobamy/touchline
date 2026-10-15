# Touchline documentation

These documents describe what Touchline is, how it is built and how it will be finished by the submission deadline of **27 October 2026, 11:59 PM Pacific Time** (about 8:00 AM on 28 October in Lagos).

| Document | Read it to learn |
| --- | --- |
| [01 Overview](01-overview.md) | The problem, who it is for, what the product does, and how it maps to the hackathon rules and judging criteria |
| [02 Plan](02-plan.md) | The day-by-day solo build plan, milestones with acceptance criteria, scope cuts, risks and stretch goals |
| [03 Architecture](03-architecture.md) | Components, data flow, Azure services, deployment topology, latency budget and security |
| [04 Data contracts](04-data-contracts.md) | The event stream, match metadata and overlay formats, with examples |
| [05 Stats engine](05-stats-engine.md) | Every metric Touchline computes and the exact formula behind it |
| [06 Agents](06-agents.md) | Agent roles, MCP tools, shared state, handoffs, failure recovery, personalization and evaluation |
| [07 Implementation guide](07-implementation.md) | Repository layout, local development, coding standards, testing, and step-by-step build notes per component |
| [08 Demo and submission](08-demo-and-submission.md) | The two-minute video script and the submission checklist |
| [09 Decisions](09-decisions.md) and [ADRs](adr/README.md) | Architecture Decision Records: every significant choice, its context, consequences and alternatives |

## Status at a glance

| Component | Status |
| --- | --- |
| Event, match and overlay contracts with JSON Schemas | Built and tested |
| Match simulator with scenarios | Built and tested |
| Stats engine | Built and tested |
| Candidate-moment rules | Built and tested |
| MCP server exposing stats tools (HTTP and stdio) | Built and tested |
| API reference: OpenAPI document and Scalar page at `/docs` | Built and tested |
| Agent orchestrator: Moment Detector, Narrator, Verifier, Director, fallbacks, schema gate | Built and tested with recorded model replies |
| Agents on Microsoft Foundry | Next: `FoundryModel` reads its settings; wiring planned |
| Event Hubs ingestion and Redis shared state | Planned |
| Pitch view and overlays | Planned |
| Deployment to Azure Container Apps | Planned |

Keep this table current as work lands.
