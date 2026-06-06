APP_NAME           := node-role-controller
YAML_FILES         := $(shell find . -type f \( -iname "*.yml" -o -iname "*.yaml" \) -not -path "./chart/templates/*")
CONFIG_FILE        ?= kind.yaml

# Versions and quality gates from .settings.yaml (single source of truth)
NODE_IMAGE         ?= $(shell yq -r '.testing.kind_node_image' .settings.yaml 2>/dev/null)
SCAN_SEVERITY      ?= $(shell yq -r '.linting.scan_severity' .settings.yaml 2>/dev/null)
COVERAGE_THRESHOLD ?= $(shell yq -r '.quality.coverage_threshold' .settings.yaml 2>/dev/null)
LINT_TIMEOUT       ?= $(shell yq -r '.quality.lint_timeout' .settings.yaml 2>/dev/null)
TEST_TIMEOUT       ?= $(shell yq -r '.quality.test_timeout' .settings.yaml 2>/dev/null)
GO_VERSION         := $(shell yq -r '.languages.go' .settings.yaml 2>/dev/null)
GOLINT_VERSION     := $(shell golangci-lint --version 2>/dev/null | awk '{print $$4}' || echo "not installed")
GORELEASER_VERSION := $(shell goreleaser --version 2>/dev/null | sed -n 's/^GitVersion:[[:space:]]*//p' || echo "not installed")
COMMIT             := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BRANCH             := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
VERSION            ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# Go
GO111MODULE     := on
CGO_ENABLED	    := 0

# Environment for Go commands
GO_ENV := \
	GO111MODULE=$(GO111MODULE) \
	CGO_ENABLED=$(CGO_ENABLED)

.PHONY: all info build lint lint-go lint-yaml clean test test-coverage bench fmt-check help tidy upgrade tag qualify helm-lint helm-publish release build bump-major bump-minor bump-patch vet

all: help

info: ## Prints the current project info
	@echo "app:        $(APP_NAME)"
	@echo "version:    $(VERSION)"
	@echo "commit:     $(COMMIT)"
	@echo "branch:     $(BRANCH)"
	@echo "go:         $(GO_VERSION)"
	@echo "linter:     $(GOLINT_VERSION)"
	@echo "goreleaser: $(GORELEASER_VERSION)"
	@echo "coverage:   $(COVERAGE_THRESHOLD)%"
	@echo "lint to:    $(LINT_TIMEOUT)"
	@echo "test to:    $(TEST_TIMEOUT)"

qualify: tidy lint test-coverage vet helm-lint ## Run all quality checks (tidy + lint + test-coverage + vet + helm-lint)

release: ## Run GoReleaser release
	goreleaser release --clean --fail-fast --timeout 30m

build: ## Run GoReleaser snapshot build (local dev)
	goreleaser build --clean --single-target --snapshot

clean: ## Clean the build artifacts
	@set -e; \
	$(GO_ENV) go clean -x; \
	rm -rf bin/$(APP_NAME) dist/ coverage.out

tidy: ## Format code and update Go module dependencies
	@set -e; \
	$(GO_ENV) go fmt ./...; \
	$(GO_ENV) go mod tidy

fmt-check: ## CI-friendly format check (no modifications)
	@test -z "$$(gofmt -l .)" || (echo "Code is not formatted. Run 'make tidy' to fix:"; gofmt -l .; exit 1)
	@echo "Code formatting check passed"

upgrade: ## Upgrades all dependencies
	@set -e; \
	$(GO_ENV) go get -u ./...; \
	$(GO_ENV) go mod tidy

lint: lint-go lint-yaml ## Lint Go and YAML

lint-go: ## Lint Go code
	@$(GO_ENV) golangci-lint -c .golangci.yaml run --modules-download-mode=readonly --timeout=$(LINT_TIMEOUT)

lint-yaml: ## Lint YAML files
	@yamllint -c .yamllint $(YAML_FILES)

test: ## Run Go tests with race detector and coverage
	@set -e; \
	GO111MODULE=$(GO111MODULE) CGO_ENABLED=1 go test -count=1 -race -timeout=$(TEST_TIMEOUT) -covermode=atomic -coverprofile=coverage.out ./...; \
	echo "Test coverage:"; \
	$(GO_ENV) go tool cover -func=coverage.out | tail -1

test-coverage: test ## Run tests and enforce coverage threshold
	@set -e; \
	coverage=$$($(GO_ENV) go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$coverage% (threshold: $(COVERAGE_THRESHOLD)%)"; \
	awk -v c="$$coverage" -v t="$(COVERAGE_THRESHOLD)" 'BEGIN { if (c+0 < t+0) exit 1 }' || \
	  (echo "ERROR: Coverage $$coverage% is below threshold $(COVERAGE_THRESHOLD)%"; exit 1); \
	echo "Coverage check passed"

bench: ## Run benchmarks
	@$(GO_ENV) go test -bench=. -benchmem ./...

vet: ## Vet the Go code
	@$(GO_ENV) go vet ./...

bump-major: ## Bump major version (1.2.3 → 2.0.0)
	tools/bump major

bump-minor: ## Bump minor version (1.2.3 → 1.3.0)
	tools/bump minor

bump-patch: ## Bump patch version (1.2.3 → 1.2.4)
	tools/bump patch

up: ## Create a Kubernetes cluster with KinD
	kind create cluster --name $(APP_NAME) --config $(CONFIG_FILE) --wait 5m

down: ## Delete a Kubernetes cluster with KinD
	kind delete cluster --name $(APP_NAME)

integration: ## Run integration tests
	@echo "Running integration tests..."
	@bash tests/integration 2

helm-lint: ## Lint the Helm chart
	@helm lint chart/

helm-publish: ## Package and push Helm chart to OCI registry
	@TAG=$${TAG:?TAG is required}; \
	sed -i.bak "s/^version:.*/version: $${TAG#v}/" chart/Chart.yaml; \
	sed -i.bak "s/^appVersion:.*/appVersion: \"$${TAG#v}\"/" chart/Chart.yaml; \
	rm -f chart/Chart.yaml.bak; \
	helm package chart/; \
	helm push node-role-controller-$${TAG#v}.tgz oci://ghcr.io/mchmarny; \
	rm -f node-role-controller-$${TAG#v}.tgz

help: ## Displays available commands
	@echo "Available make targets:"; \
	grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk \
		'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
