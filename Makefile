.PHONY: dev down proto build test lint seed clean

# --- Development ---

dev: ## Start all infrastructure services
	docker compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@docker compose ps
	@echo ""
	@echo "Services:"
	@echo "  PostgreSQL:       localhost:5432"
	@echo "  Redis:            localhost:6379"
	@echo "  Neo4j Browser:    http://localhost:7474"
	@echo "  Neo4j Bolt:       localhost:7687"
	@echo "  SpiceDB gRPC:     localhost:50051"
	@echo "  SpiceDB HTTP:     http://localhost:8443"
	@echo "  Redpanda Kafka:   localhost:19092"
	@echo "  Redpanda Console: http://localhost:8080"

down: ## Stop all services
	docker compose down

down-clean: ## Stop all services and remove volumes
	docker compose down -v

# --- Protobuf ---

proto: ## Generate code from protobuf schemas
	buf lint
	buf generate
	@echo "Proto generation complete → gen/"

proto-lint: ## Lint protobuf schemas
	buf lint

# --- Go Services ---

build: ## Build all Go services
	@for svc in authz-engine entity-service trust-engine decision-service federation-service surplus-engine; do \
		echo "Building $$svc..."; \
		cd services/$$svc && go build ./... && cd ../..; \
	done
	@echo "All services built"

test: ## Run all tests
	@echo "=== Go Tests ==="
	@for svc in authz-engine entity-service trust-engine decision-service federation-service surplus-engine; do \
		if [ -f "services/$$svc/go.mod" ]; then \
			echo "Testing $$svc..."; \
			cd services/$$svc && go test ./... -v && cd ../..; \
		fi \
	done
	@echo ""
	@echo "=== TypeScript Tests ==="
	@if [ -f "services/api-gateway/package.json" ]; then \
		cd services/api-gateway && pnpm test; \
	fi

lint: ## Run linters
	buf lint
	@for svc in authz-engine entity-service trust-engine decision-service federation-service surplus-engine; do \
		if [ -f "services/$$svc/go.mod" ]; then \
			cd services/$$svc && go vet ./... && cd ../..; \
		fi \
	done

# --- Data ---

seed: ## Seed demo data (entities, relationships, trust scores)
	@echo "Seeding demo data..."
	@if [ -f "scripts/seed-data.sh" ]; then bash scripts/seed-data.sh; fi

# --- Utilities ---

clean: ## Clean build artifacts
	rm -rf gen/
	@for svc in authz-engine entity-service trust-engine decision-service federation-service surplus-engine; do \
		rm -f services/$$svc/$$svc; \
	done

logs: ## Tail all service logs
	docker compose logs -f

ps: ## Show running services
	docker compose ps

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
