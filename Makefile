.PHONY: test vet build run demo

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
