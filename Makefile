DB_URL ?= postgresql://unibo_user:unibo_pass@localhost:5432/unibo_toolkit?sslmode=disable
MIGRATIONS_DIR ?= ../databases/migrations

.PHONY: setup dev-up dev-down dev migrate-up migrate-down sqlc build test lint clean

setup:
	go mod download
	cp -n .env.example .env || true

dev-up:
	docker compose -f docker-compose.dev.yaml up -d
	@until docker compose -f docker-compose.dev.yaml exec postgres pg_isready -U unibo_user; do sleep 1; done

dev-down:
	docker compose -f docker-compose.dev.yaml down -v

dev:
	go run ./cmd/main/main.go

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_URL)" down 1

sqlc:
	cd internal/storage && sqlc generate

build:
	CGO_ENABLED=0 go build -o bin/calendar-manager ./cmd/main/main.go

test:
	go test -v -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out
