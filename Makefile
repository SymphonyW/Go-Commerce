.PHONY: test test-unit test-integration test-e2e test-integration-up test-e2e-up

COMPOSE ?= docker compose

test: test-unit

test-unit:
	go test ./...

test-integration-up:
	$(COMPOSE) up -d mysql redis rabbitmq

test-integration: test-integration-up
	go test ./... -tags=integration

test-e2e-up:
	$(COMPOSE) up -d --build

test-e2e: test-e2e-up
	go test ./tests/e2e -tags=e2e -v
