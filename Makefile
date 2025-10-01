# Project metadata
MODULE           := github.com/rvazquez/ai-assisted-observability-poc/go
BIN_DIR          := bin
COVER_PROFILE    := coverage.out
CMD_INGEST       := ./cmd/ingest
CMD_MCP          := ./cmd/mcp-server

# Container metadata
IMAGE_REGISTRY   ?= quay.io/roivaz
IMAGE_NAME       ?= aro-hcp-go
IMAGE_TAG        ?= latest
IMAGE            := $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Kind / Kubernetes configuration
KIND_VERSION           ?= v0.27.0
KIND_NODE_VERSION      ?= v1.27.10
CLOUD_PROVIDER_KIND_VERSION ?= latest
LOCALBIN               := $(shell pwd)/$(BIN_DIR)
KIND                   := $(LOCALBIN)/kind
CLOUD_PROVIDER_KIND    := $(LOCALBIN)/cloud-provider-kind
CLOUD_PROVIDER_KIND_PID_FILE := /tmp/cloud-provider-kind.pid
CLOUD_PROVIDER_KIND_LOG_FILE := $(PWD)/cloud-provider-kind.log
KUBECONFIG             ?= $(PWD)/kubeconfig

# Tools
GO          ?= go
CONTAINER   ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null || echo "docker")

.DEFAULT_GOAL := help

help:
	@grep -E '^[a-zA-Z0-9_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "%-24s %s\n", $$1, $$2}'
.PHONY: help

fmt: ## Format Go code
	$(GO) fmt ./...
.PHONY: fmt

lint: ## Run go vet and staticcheck (if available)
	$(GO) vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; else echo "staticcheck not installed"; fi
.PHONY: lint

test: ## Run unit tests with coverage
	$(GO) test ./... -coverprofile $(COVER_PROFILE)
.PHONY: test

build: ## Build binaries (ingest + mcp-server)
	$(GO) build $(CMD_INGEST)
	$(GO) build $(CMD_MCP)
.PHONY: build

run-ingest-prs: ## Run ingest command locally
	$(GO) run $(CMD_INGEST) prs
.PHONY: run-ingest-prs

run-ingest-docs: ## Run ingest command locally
	$(GO) run $(CMD_INGEST) docs
.PHONY: run-ingest-docs

run-mcp: ## Run MCP server locally
	$(GO) run $(CMD_MCP)
.PHONY: run-mcp

clean: ## Remove build artifacts
	rm -f $(COVER_PROFILE)
	$(GO) clean ./...
.PHONY: clean

tidy: ## Update module dependencies
	$(GO) mod tidy
.PHONY: tidy

bin-dir:
	@mkdir -p $(LOCALBIN)
.PHONY: bin-dir

$(KIND): bin-dir ## Download kind locally if necessary
	@echo "Downloading kind $(KIND_VERSION)..."
	GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/kind@$(KIND_VERSION)
.PHONY: $(KIND)

$(CLOUD_PROVIDER_KIND): bin-dir ## Download cloud-provider-kind locally if necessary
	@echo "Downloading cloud-provider-kind $(CLOUD_PROVIDER_KIND_VERSION)..."
	GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/cloud-provider-kind@$(CLOUD_PROVIDER_KIND_VERSION)
.PHONY: $(CLOUD_PROVIDER_KIND)

container-build: ## Build container image
	$(CONTAINER) build -t $(IMAGE) .
.PHONY: container-build

container-run: ## Run container locally
	$(CONTAINER) run --rm -it --env-file ../manifests/config.env $(IMAGE)
.PHONY: container-run

container-push: ## Push container image to registry
	$(CONTAINER) push $(IMAGE)
.PHONY: container-push

cloud-provider-kind-start: ## Start cloud-provider-kind in background
	hack/cloud-provider-kind.sh start
.PHONY: cloud-provider-kind-start

cloud-provider-kind-stop: ## Stop cloud-provider-kind
	hack/cloud-provider-kind.sh stop
.PHONY: cloud-provider-kind-stop

kind-create: export KUBECONFIG := $(KUBECONFIG)
kind-create: container-build $(KIND) $(CLOUD_PROVIDER_KIND) cloud-provider-kind-start ## Create kind cluster and load image
	$(KIND) create cluster --wait 5m --image kindest/node:$(KIND_NODE_VERSION)
	$(MAKE) kind-load-image
.PHONY: kind-create

kind-delete: export KUBECONFIG := $(KUBECONFIG)
kind-delete: $(KIND) cloud-provider-kind-stop ## Delete kind cluster
	$(KIND) delete cluster
.PHONY: kind-delete

kind-load-image: export KUBECONFIG := $(KUBECONFIG)
kind-load-image: $(KIND) container-build ## Load container image into kind
	tmpfile=$$(mktemp) && \
		$(CONTAINER) save -o $$tmpfile $(IMAGE) && \
		$(KIND) load image-archive $$tmpfile --name kind && \
		rm -f $$tmpfile
.PHONY: kind-load-image

k8s-deploy: ## Deploy Kubernetes resources
	kubectl apply -k ../manifests
.PHONY: k8s-deploy

k8s-undeploy: ## Remove Kubernetes resources
	kubectl delete -k ../manifests
.PHONY: k8s-undeploy

k8s-logs: ## Tail logs from embedder job
	POD=$$(kubectl get pods -l app=aro-hcp-embedder --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null) && \
	if [ -n "$$POD" ]; then kubectl logs $$POD; else echo "No embedder pods found"; fi
.PHONY: k8s-logs

db-status: ## Check PostgreSQL connection
	$(GO) run ./cmd/dbstatus
.PHONY: db-status

