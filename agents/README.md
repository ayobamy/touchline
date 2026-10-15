# Agents (planned)

The Python orchestrator and the four Touchline agents — Moment Detector, Narrator, Verifier and Director — built with Microsoft Agent Framework and hosted on Microsoft Foundry.

Planned layout:

```
agents/
├── instructions/     one Markdown file of system instructions per agent
├── touchline_agents/ orchestrator, handoffs, Director scheduling, overlay publisher
└── tests/            Verifier, Director and fallback tests using recorded model output
```

Design: [docs/06-agents.md](../docs/06-agents.md). Build steps: [docs/07-implementation.md](../docs/07-implementation.md#6-agents).
