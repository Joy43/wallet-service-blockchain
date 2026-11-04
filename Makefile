.PHONY: tidy run test lint build keygen up down migrate-up migrate-down proto k8s-image k8s-secret k8s-apply k8s-teardown

DB_URL ?= postgres://auth:auth@localhost:5432/auth?sslmode=disable
K8S_NS ?= platform
IMAGE ?= yourorg/auth-service:latest
AUTH := services/auth
MIGRATIONS := $(wildcard $(AUTH)/migrations/0*.sql)

tidy:
	cd gen && go mod tidy
	cd $(AUTH) && go mod tidy

run:
	set -a; . ./.env; set +a; cd $(AUTH) && go run ./cmd/auth

test:
	cd $(AUTH) && go test -race -cover ./...

lint:
	cd $(AUTH) && go vet ./...

build:
	cd $(AUTH) && CGO_ENABLED=0 go build -o ../../bin/auth ./cmd/auth

# Prints a new Ed25519 signing key: put it in .env (local) or a secret manager (real environments).
keygen:
	@cd $(AUTH) && go run ./cmd/keygen

up:
	docker compose up --build

down:
	docker compose down -v

migrate-up:
	migrate -path $(AUTH)/migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path $(AUTH)/migrations -database "$(DB_URL)" down 1

# Regenerate gen/auth/v1 (needs buf, protoc-gen-go, protoc-gen-go-grpc on PATH)
proto:
	buf generate

# ---- Kubernetes (namespace "platform") ----
k8s-image:
	docker build -f $(AUTH)/Dockerfile -t $(IMAGE) .

# Creates app-secrets with random values. Refuses to overwrite an existing Secret.
k8s-secret:
	kubectl apply -f deploy/k8s/base/namespace.yaml
	@if kubectl -n $(K8S_NS) get secret app-secrets >/dev/null 2>&1; then \
		echo "app-secrets already exists; remove it first to regenerate"; exit 1; fi
	@PW=$$(openssl rand -hex 16); \
	KEY=$$(cd $(AUTH) && go run ./cmd/keygen | cut -d= -f2-); \
	kubectl -n $(K8S_NS) create secret generic app-secrets \
		--from-literal=POSTGRES_PASSWORD=$$PW \
		--from-literal=DATABASE_URL="postgres://auth:$$PW@postgres:5432/auth?sslmode=disable" \
		--from-literal=JWT_PRIVATE_KEY=$$KEY

# The migrate Job is immutable, so it is recreated on every apply.
k8s-apply:
	kubectl apply -f deploy/k8s/base/namespace.yaml
	kubectl -n $(K8S_NS) create configmap migrations $(addprefix --from-file=,$(MIGRATIONS)) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -k deploy/k8s/base
	kubectl -n $(K8S_NS) delete job migrate --ignore-not-found
	kubectl apply -f deploy/k8s/base/migrate-job.yaml

k8s-teardown:
	kubectl delete -k deploy/k8s/base --ignore-not-found
	kubectl -n $(K8S_NS) delete job migrate --ignore-not-found
