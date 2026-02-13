.PHONY: tidy test build-server build-worker build-tx-indexer infra-up infra-down run-server run-worker run-tx-indexer deploy-prod deploy-configs deploy-server deploy-worker deploy-tx-indexer

tidy:
	go mod tidy

test:
	go test ./...

build-server:
	go build -o bin/server ./cmd/server/

build-worker:
	go build -o bin/worker ./cmd/worker/

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

run-tx-indexer:
	go run ./cmd/tx_indexer/

NS ?= plugin-developer

deploy-prod: deploy-configs deploy-server deploy-worker deploy-tx-indexer

deploy-configs:
	kubectl -n $(NS) apply -f deploy/prod

deploy-server:
	kubectl -n $(NS) apply -f deploy/01_server.yaml
	kubectl -n $(NS) rollout status deployment/server --timeout=300s

deploy-worker:
	kubectl -n $(NS) apply -f deploy/01_worker.yaml
	kubectl -n $(NS) rollout status deployment/worker --timeout=300s

deploy-tx-indexer:
	kubectl -n $(NS) apply -f deploy/01_tx_indexer.yaml
	kubectl -n $(NS) rollout status deployment/tx-indexer --timeout=300s
