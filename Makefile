.PHONY: tidy run test lint build up down migrate-up migrate-down proto

DB_URL ?= postgres://orders:orders@localhost:5432/orders?sslmode=disable

tidy:
	go mod tidy

run:
	set -a; . ./.env; set +a; go run ./cmd/server

test:
	go test -race -cover ./...

lint:
	go vet ./...

build:
	CGO_ENABLED=0 go build -o bin/server ./cmd/server

up:
	docker compose up --build

down:
	docker compose down -v

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

proto:
	protoc --go_out=. --go_opt=module=github.com/yourorg/order-service \
	       --go-grpc_out=. --go-grpc_opt=module=github.com/yourorg/order-service \
	       proto/order/v1/order.proto
