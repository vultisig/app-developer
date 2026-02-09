.PHONY: tidy test build-server build-worker build-syncer build-tx-indexer infra-up infra-down run-server run-worker run-syncer run-tx-indexer

tidy:
	go mod tidy

test:
	go test ./...

build-server:
	go build -o bin/server ./cmd/server/

build-worker:
	go build -o bin/worker ./cmd/worker/

build-syncer:
	go build -o bin/syncer ./cmd/syncer/

build-tx-indexer:
	go build -o bin/tx_indexer ./cmd/tx_indexer/

infra-up:
	docker compose up -d

infra-down:
	docker compose down

run-server:
	go run ./cmd/server/

run-worker:
	go run ./cmd/worker/

run-syncer:
	go run ./cmd/syncer/

run-tx-indexer:
	go run ./cmd/tx_indexer/
