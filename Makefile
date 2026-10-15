.PHONY: test vet build run demo serve agents agents-test

# Replay speed for `make serve`: 60 = one match minute per second, 1 = real time.
SPEED ?= 60
PYTHON ?= python3
VENV := agents/.venv

test: vet
	go test ./...

vet:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	go vet ./...

build:
	go build -o bin/ ./cmd/...

# One match, seed 7, as fast as possible.
run:
	go run ./cmd/simulator -seed 7 -out match.ndjson
	@echo "wrote match.ndjson"

# The late-press story at one match minute per second, with stats every 5 minutes.
demo:
	go run ./cmd/simulator -seed 7 -scenario scenarios/late-press.json -speed 60 \
		| go run ./cmd/stats -every 5 -pretty

# Serve the late-press match: MCP at http://localhost:8080/mcp, API reference at /docs.
serve:
	go run ./cmd/server -simulate -seed 7 -scenario scenarios/late-press.json -speed $(SPEED)

# Python environment for agents/, rebuilt when the pinned requirements change.
$(VENV)/.installed: agents/requirements-dev.txt
	$(PYTHON) -m venv $(VENV)
	$(VENV)/bin/pip install --quiet --require-hashes -r agents/requirements-dev.txt
	touch $@

# Lint, type-check and test the agents, with coverage. Needs Python 3.12+ (and Go
# for the integration tests, which start the server themselves).
agents-test: $(VENV)/.installed
	cd agents && .venv/bin/ruff check . && .venv/bin/ruff format --check .
	cd agents && .venv/bin/mypy
	cd agents && .venv/bin/coverage run -m pytest && .venv/bin/coverage report

# Run the agents end to end with recorded model replies: starts the server,
# replays seed 7 late-press and prints the agent trace. No Azure account needed.
agents: $(VENV)/.installed
	@mkdir -p bin
	cd agents && .venv/bin/python -m touchline_agents --replay --out ../bin/overlays.ndjson
