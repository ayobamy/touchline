# Agents

The Touchline orchestrator and its four agents: Moment Detector, Narrator, Verifier and
Director. It polls candidate moments from the Touchline server over MCP, runs each one through
the agents with a deadline per step, and publishes overlays that pass
`schema/overlay.schema.json`.

Design: [docs/06-agents.md](../docs/06-agents.md). How it is built and run:
[docs/07-implementation.md](../docs/07-implementation.md#6-agents-skeleton-built-foundry-models-to-wire).
Decisions: ADRs [0021](../docs/adr/0021-official-mcp-python-sdk-client.md) to
[0024](../docs/adr/0024-director-and-fallbacks-in-code.md).

## Run it

From the repository root, with Python 3.12+ and Go 1.24+:

```bash
make agents        # replay seed 7 late-press with recorded model replies; trace on stderr
make agents-test   # ruff, mypy --strict, pytest with coverage
```

Both create `agents/.venv` from the hashed lock file. To run by hand:

```bash
cd agents
python3 -m venv .venv
.venv/bin/pip install --require-hashes -r requirements-dev.txt
.venv/bin/python -m touchline_agents --help
```

No Azure account is needed: the default model is `FakeModel`, which replays
`fixtures/late_press_model.json`. `--model foundry` reads `FOUNDRY_PROJECT_ENDPOINT` and
`FOUNDRY_{MOMENT_DETECTOR,NARRATOR,VERIFIER}_DEPLOYMENT`; the Foundry client itself is not wired
yet, so every agent call fails and templates are published.

## Layout

```text
agents/
├── instructions/        one Markdown file of instructions per agent
├── touchline_agents/    orchestrator, agents, Director, templates, MCP client, model interface
├── fixtures/            recorded model replies and recorded tool results (seed 7 late-press)
├── tests/               unit tests (no network) and an integration test (starts the Go server)
├── pyproject.toml       dependencies, pinned exactly, and tool settings
└── requirements*.txt    every transitive dependency, pinned with hashes
```

## Dependencies

Runtime: `mcp` (the official MCP SDK) and `jsonschema`. Development: `pytest`, `ruff`, `mypy`,
`coverage`, `types-jsonschema`. After changing a pin in `pyproject.toml`, recompile the locks:

```bash
uv pip compile pyproject.toml --python-version 3.12 --generate-hashes --no-header -o requirements.txt
uv pip compile pyproject.toml --group dev --python-version 3.12 --generate-hashes --no-header -o requirements-dev.txt
```
