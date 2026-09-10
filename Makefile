COMPOSE := docker compose -f deploy/compose/docker-compose.yml

.PHONY: dev compose-up compose-down migrate test lint run-catalog

dev: compose-up

compose-up:
	$(COMPOSE) up -d
	$(COMPOSE) --profile migrate run --rm migrate

compose-down:
	$(COMPOSE) down

migrate:
	$(COMPOSE) --profile migrate run --rm migrate

test:
	go test -v ./...

lint:
	go vet ./...

run-catalog:
	go run ./cmd/catalog
