COMPOSE := docker compose -f deploy/compose/docker-compose.yml

.PHONY: dev compose-up compose-down test lint run-catalog

dev: compose-up

compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

test:
	go test -v ./...

lint:
	go vet ./...

run-catalog:
	go run ./cmd/catalog
