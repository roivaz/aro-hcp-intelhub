.PHONY: help install install-dev clean clean-all venv run run-mcp-http setup-cursor-mcp setup-cursor-mcp-legacy test lint format check-format check-deps check \
        container-build container-run container-push kind kind-create kind-delete kind-load-image \
        cloud-provider-kind cloud-provider-kind-start cloud-provider-kind-stop \
        k8s-deploy k8s-deploy-postgresql k8s-logs k8s-undeploy db-status freeze info shell type-check try
.DEFAULT_GOAL := help

# Variables
LOCALBIN := $(shell pwd)/bin
PYTHON := python3.12
VENV_DIR := .venv
VENV_BIN := $(VENV_DIR)/bin
PYTHON_VENV := $(VENV_BIN)/python
PIP_VENV := $(VENV_BIN)/pip
REQUIREMENTS := requirements.txt
DEV_REQUIREMENTS := requirements-dev.txt

IMAGE_REGISTRY := quay.io/roivaz
IMAGE_NAME := aro-hcp-embedder
IMAGE_TAG := latest
IMAGE := $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Container tool auto-discovery (prefer podman over docker)
CONTAINER_TOOL := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null || echo "docker")

help: ## Show this help message
	@grep -E '^[a-zA-Z0-9_-]+:.*## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "%-20s %s\n", $$1, $$2}'

bin: $(LOCALBIN)
$(LOCALBIN):
	@echo "Creating local bin directory..."
	mkdir -p $(LOCALBIN)
	@echo "Local bin directory created at $(LOCALBIN)"

venv: $(VENV_DIR)/pyvenv.cfg ## Create virtual environment
$(VENV_DIR)/pyvenv.cfg:
	@echo "Creating virtual environment..."
	$(PYTHON) -m venv --copies $(VENV_DIR)
	@echo "Virtual environment created at $(VENV_DIR)"

check-deps: ## Check system dependencies
	@./hack/check-deps.sh

install: venv check-deps ## Install production dependencies
	@echo "Installing production dependencies..."
	$(PIP_VENV) install --upgrade pip setuptools wheel
	$(PIP_VENV) install -r $(REQUIREMENTS)
	@echo "Production dependencies installed"

install-dev: venv check-deps ## Install development dependencies
	@echo "Installing development dependencies..."
	$(PIP_VENV) install --upgrade pip setuptools wheel
	$(PIP_VENV) install -r $(REQUIREMENTS)
	@if [ -f $(DEV_REQUIREMENTS) ]; then \
		$(PIP_VENV) install -r $(DEV_REQUIREMENTS); \
	else \
		$(PIP_VENV) install black flake8 pytest pytest-cov mypy; \
	fi
	@echo "Development dependencies installed"

run-generator: venv ## Run the ARO-HCP embedder
	@echo "Starting ARO-HCP embedder..."
	@if [ ! -f manifests/config.env ]; then \
		echo "Warning: manifests/config.env file not found. Please configure manifests/config.env."; \
		echo "Continuing with default environment variables..."; \
	else \
		echo "Loading configuration from manifests/config.env..."; \
		export $$(grep -v '^#' manifests/config.env | xargs); \
	fi; \
	$(PYTHON_VENV) embedding_generator.py

run: venv ## Start HTTP/SSE MCP server (recommended for production)
	@echo "🌐 Starting ARO-HCP HTTP/SSE MCP server..."
	@if [ ! -f manifests/config.env ]; then \
		echo "Warning: manifests/config.env file not found. Please configure manifests/config.env."; \
		echo "Continuing with default environment variables..."; \
	else \
		echo "Loading configuration from manifests/config.env..."; \
		export $$(grep -v '^#' manifests/config.env | xargs); \
	fi; \
	$(PYTHON_VENV) mcp_server.py

lint: venv ## Check code quality with flake8
	@echo "Checking code quality..."
	@if command -v $(VENV_BIN)/flake8 >/dev/null 2>&1; then \
		$(VENV_BIN)/flake8 embedding_generator.py --max-line-length=100 --ignore=E203,W503; \
		echo "Code quality check passed"; \
	else \
		echo "flake8 not installed. Run 'make install-dev' first"; \
		exit 1; \
	fi

format: venv ## Format code with black
	@echo "Formatting code..."
	@if command -v $(VENV_BIN)/black >/dev/null 2>&1; then \
		$(VENV_BIN)/black embedding_generator.py --line-length=100; \
		echo "Code formatting complete"; \
	else \
		echo "black not installed. Run 'make install-dev' first"; \
		exit 1; \
	fi

check-format: venv ## Check if code is properly formatted
	@echo "Checking code formatting..."
	@if command -v $(VENV_BIN)/black >/dev/null 2>&1; then \
		$(VENV_BIN)/black embedding_generator.py --line-length=100 --check --diff; \
		echo "Code is properly formatted"; \
	else \
		echo "black not installed. Run 'make install-dev' first"; \
		exit 1; \
	fi

type-check: venv ## Run type checking with mypy
	@echo "Running type checks..."
	@if command -v $(VENV_BIN)/mypy >/dev/null 2>&1; then \
		$(VENV_BIN)/mypy embedding_generator.py --ignore-missing-imports; \
		echo "Type checking complete"; \
	else \
		echo "mypy not installed. Run 'make install-dev' first"; \
		exit 1; \
	fi

check: check-format lint type-check ## Run all code quality checks

clean: ## Remove Python cache files and __pycache__ directories
	@echo "Cleaning Python cache files..."
	find . -type f -name "*.pyc" -delete
	find . -type d -name "__pycache__" -delete
	find . -type d -name "*.egg-info" -exec rm -rf {} +
	find . -type f -name ".coverage" -delete
	rm -rf .pytest_cache/
	rm -rf htmlcov/
	rm -rf dist/
	rm -rf build/
	@echo "Cache files cleaned"

clean-all: clean ## Remove virtual environment and all generated files
	@echo "Removing virtual environment..."
	rm -rf $(VENV_DIR)
	rm -rf aro-hcp-repo/
	@echo "Complete cleanup finished"

freeze: venv ## Generate current package versions
	@echo "Generating requirements.txt from current environment..."
	$(PIP_VENV) freeze > requirements-frozen.txt
	@echo "Requirements saved to requirements-frozen.txt"

shell: venv ## Activate virtual environment shell
	@echo "Activating virtual environment..."
	@echo "Run 'deactivate' to exit the virtual environment"
	@bash --init-file <(echo "source $(VENV_BIN)/activate; echo 'Virtual environment activated'")

info: ## Show project information
	@echo "ARO-HCP Embedder Project Information"
	@echo "Python version: $(shell $(PYTHON) --version)"
	@echo "Virtual environment: $(VENV_DIR)"
	@if [ -d $(VENV_DIR) ]; then \
		echo "Virtual environment status: Created"; \
		echo "Virtual environment Python: $(shell $(PYTHON_VENV) --version 2>/dev/null || echo 'Not available')"; \
	else \
		echo "Virtual environment status: Not created"; \
	fi
	@echo "Requirements file: $(REQUIREMENTS)"
	@if [ -f manifests/config.env ]; then \
		echo "Environment file: manifests/config.env exists"; \
	else \
		echo "Environment file: manifests/config.env missing (please configure it)"; \
	fi

# Container targets
container-build: ## Build container image
	@echo "Building container image with $(CONTAINER_TOOL)..."
	$(CONTAINER_TOOL) build -t $(IMAGE) .
	@echo "Container image built: $(IMAGE)"

container-run: ## Run container locally
	@echo "Running container with $(CONTAINER_TOOL)..."
	@if [ ! -f manifests/config.env ]; then \
		echo "Warning: manifests/config.env file not found. Please configure manifests/config.env."; \
		echo "Continuing with default environment variables..."; \
	fi
	$(CONTAINER_TOOL) run --rm -it --env-file manifests/config.env $(IMAGE)

container-push: ## Push container image to registry
	@echo "Pushing container image to registry with $(CONTAINER_TOOL)..."
	$(CONTAINER_TOOL) tag $(IMAGE)
	$(CONTAINER_TOOL) push $(IMAGE)
	@echo "Image pushed to $(IMAGE)"

# Kubernetes targets
KIND ?= $(LOCALBIN)/kind
KIND_VERSION ?= v0.27.0
KIND_NODE_VERSION := v1.27.10
CLOUD_PROVIDER_KIND ?= $(LOCALBIN)/cloud-provider-kind
CLOUD_PROVIDER_KIND_VERSION ?= latest
CLOUD_PROVIDER_KIND_PID_FILE := /tmp/cloud-provider-kind.pid
CLOUD_PROVIDER_KIND_LOG_FILE := $(PWD)/cloud-provider-kind.log

kind: $(KIND) ## Download kind locally if necessary
$(KIND):
	@mkdir -p $(LOCALBIN)
	@echo "Downloading kind@$(KIND_VERSION)..."
	@GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION)

cloud-provider-kind: $(CLOUD_PROVIDER_KIND) ## Download cloud-provider-kind locally if necessary
$(CLOUD_PROVIDER_KIND):
	@mkdir -p $(LOCALBIN)
	@echo "Downloading cloud-provider-kind@$(CLOUD_PROVIDER_KIND_VERSION)..."
	@GOBIN=$(LOCALBIN) go install sigs.k8s.io/cloud-provider-kind@$(CLOUD_PROVIDER_KIND_VERSION)

cloud-provider-kind-start: ## Start cloud-provider-kind in background
	@./hack/cloud-provider-kind.sh start

cloud-provider-kind-stop: ## Stop cloud-provider-kind running in background
	@./hack/cloud-provider-kind.sh stop

kind-create: export KUBECONFIG = $(PWD)/kubeconfig
kind-create: container-build kind cloud-provider-kind-start ## Runs a k8s kind cluster
	$(KIND) create cluster --wait 5m --image kindest/node:$(KIND_NODE_VERSION)
	$(MAKE) kind-load-image

kind-delete: ## Deletes the kind cluster and the registry
kind-delete: kind cloud-provider-kind-stop
	$(KIND) delete cluster

kind-load-image: export KUBECONFIG = $(PWD)/kubeconfig
kind-load-image: kind container-build ## Load the container image into the cluster
	tmpfile=$$(mktemp) && \
		$(CONTAINER_TOOL) save -o $${tmpfile} $(IMAGE) && \
		$(KIND) load image-archive $${tmpfile} --name kind && \
		rm -f $${tmpfile}

k8s-deploy: ## Deploy Kubernetes resources (embedder + PostgreSQL)
	@echo "Deploying Kubernetes resources..."
	kubectl apply -k manifests/
	@echo "Kubernetes resources deployed"


k8s-undeploy: ## Deploy Kubernetes secret for database credentials
	@echo "Undeploying Kubernetes resources..."
	kubectl delete -k manifests/
	@echo "Kubernetes resources undeployed"

k8s-logs: ## View logs from the most recent embedder job
	@echo "Fetching logs from most recent job..."
	@POD=$$(kubectl get pods -l app=aro-hcp-embedder --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null) && \
	if [ -n "$$POD" ]; then \
		echo "Showing logs for pod: $$POD"; \
		kubectl logs $$POD; \
	else \
		echo "No embedder pods found"; \
	fi

# Database management targets
db-status: ## Check PostgreSQL connection and show connection info
	@echo "PostgreSQL Connection Status:"
	@echo "============================="
	@LB_IP=$$(kubectl get svc postgresql -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo ""); \
	if [ -n "$$LB_IP" ]; then \
		echo "🌐 LoadBalancer detected - External IP: $$LB_IP"; \
		echo "📍 Connection: postgresql://postgres:postgres@$$LB_IP:5432/aro_hcp_embeddings"; \
		echo ""; \
		echo "Testing connection to LoadBalancer..."; \
		$(PYTHON_VENV) -c "import psycopg; conn = psycopg.connect(host='$$LB_IP', port=5432, dbname='aro_hcp_embeddings', user='postgres', password='postgres'); print('✅ Database connection successful via LoadBalancer'); conn.close()" 2>/dev/null || \
		$(PYTHON_VENV) -c "print('❌ Database connection failed via LoadBalancer')"; \
	else \
		echo "🏠 Local/Environment connection"; \
		$(PYTHON_VENV) -c "import os, psycopg; from dotenv import load_dotenv; load_dotenv('manifests/config.env'); host = os.getenv('POSTGRES_HOST', 'localhost'); port = int(os.getenv('POSTGRES_PORT', '5432')); dbname = os.getenv('POSTGRES_DB', 'aro_hcp_embeddings'); user = os.getenv('POSTGRES_USER', 'postgres'); password = os.getenv('POSTGRES_PASSWORD', 'postgres'); print(f'📍 Connection: postgresql://{user}:{password}@{host}:{port}/{dbname}'); print(''); print('Testing connection...')"; \
		$(PYTHON_VENV) -c "import os, psycopg; from dotenv import load_dotenv; load_dotenv('manifests/config.env'); host = os.getenv('POSTGRES_HOST', 'localhost'); port = int(os.getenv('POSTGRES_PORT', '5432')); dbname = os.getenv('POSTGRES_DB', 'aro_hcp_embeddings'); user = os.getenv('POSTGRES_USER', 'postgres'); password = os.getenv('POSTGRES_PASSWORD', 'postgres'); conn = psycopg.connect(host=host, port=port, dbname=dbname, user=user, password=password); print('✅ Database connection successful'); conn.close()" 2>/dev/null || \
		$(PYTHON_VENV) -c "print('❌ Database connection failed')"; \
	fi
# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef