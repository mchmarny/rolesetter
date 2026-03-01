APP_NAME           := node-role-controller
YAML_FILES         := $(shell find . -type f \( -iname "*.yml" -o -iname "*.yaml" \) -not -path "./chart/templates/*")
CONFIG_FILE        ?= kind.yaml

# Versions from .settings.yaml (single source of truth)
NODE_IMAGE         ?= $(shell yq -r '.testing.kind_node_image' .settings.yaml 2>/dev/null)
SCAN_SEVERITY      ?= $(shell yq -r '.linting.scan_severity' .settings.yaml 2>/dev/null)

# Go
GO111MODULE     := on
CGO_ENABLED	    := 0

# Environment for Go commands
GO_ENV := \
	GO111MODULE=$(GO111MODULE) \
	CGO_ENABLED=$(CGO_ENABLED)

.PHONY: all build lint clean test help tidy upgrade tag pre helm-lint helm-publish release build-snapshot bump-major bump-minor bump-patch

all: help

pre: tidy lint test vet helm-lint ## Run all quality checks

build: ## Build the Go binary locally
	$(GO_ENV) go build -v -o bin/$(APP_NAME) main.go

release: ## Run GoReleaser release
	goreleaser release --clean --fail-fast --timeout 30m

build-snapshot: ## Run GoReleaser snapshot build (local dev)
	goreleaser build --clean --single-target --snapshot

clean: ## Clean the build artifacts
	$(GO_ENV) go clean -x; \
	rm -f bin/$(APP_NAME)

tidy: ## Run go mod tidy in src
	$(GO_ENV) go fmt ./...; \
	$(GO_ENV) go mod tidy

upgrade: ## Upgrades all dependencies
	$(GO_ENV) go get -u ./...; \
	$(GO_ENV) go mod tidy;

lint: ## Lint the Go code and YAML files
	$(GO_ENV) golangci-lint -c .golangci.yaml run --modules-download-mode=readonly; \
	yamllint -c .yamllint $(YAML_FILES)

test: ## Run Go tests and generate coverage report
	$(GO_ENV) go test -count=1 -covermode=atomic -coverprofile=coverage.out ./... || exit 1; \
	echo "Generating coverage report..."; \
	$(GO_ENV) go tool cover -func=coverage.out

vet: ## Vet the Go code
	$(GO_ENV) go vet ./...

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
	@echo "Running integration tests..."; \
	bash tests/integration 2 || exit 1;

helm-lint: ## Lint the Helm chart
	helm lint chart/

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
		'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

