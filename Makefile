.DEFAULT_GOAL := help

REPO       ?= quay.io/aswinsuryan/skynet
VERSION    ?= latest
PLATFORMS  ?= linux/amd64,linux/arm64
CLUSTERS   ?= cluster1 cluster2
CLUSTER    ?= cluster1
CONTEXT    ?= kind-cluster1

# OVN-Kubernetes source — used by scripts/kind/create-clusters.sh.
# contrib/kind.sh is cloned at depth=1 from master so the script and
# image always match.  Override OVN_K_DIR to skip the clone entirely.
OVN_K_REPO   ?= https://github.com/ovn-kubernetes/ovn-kubernetes.git
OVN_K_BRANCH ?= master
OVN_K_DIR    ?= /opt/ovn-kubernetes
OVN_IMAGE    ?= ghcr.io/ovn-kubernetes/ovn-kubernetes/ovn-kube-ubuntu:master
WORKERS      ?= 2

# Setup container image — bundles kind/kubectl/git/jq/jinjanate so developers
# only need Docker + Go.  All KIND and deploy targets run inside this container.
SETUP_IMAGE  ?= mcn-setup:latest

# Common docker run flags reused by all container targets.
DOCKER_RUN = docker run --rm \
	--network host \
	--security-opt label=disable \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-v $(CURDIR):/workspace \
	-v /tmp:/tmp \
	-e CLUSTERS="$(CLUSTERS)"

# ── Help ──────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help message
	@echo 'SkyNet - Multi-Cluster Networking for OVN-Kubernetes'
	@echo ''
	@echo 'Usage: make <target>'
	@echo ''
	@echo 'Quick Start (Recommended):'
	@echo '  make deploy        - Full deployment: create clusters + build + deploy'
	@echo '  make e2e           - Run end-to-end tests'
	@echo '  make verify-bgp    - Verify BGP peering status'
	@echo '  make clean         - Clean up test environment'
	@echo ''
	@echo 'Step-by-step:'
	@echo '  make clusters      - Create 2 OVN-K KIND clusters with FRR-K8s'
	@echo '  make build-agent   - Build and load SkyNet agent image'
	@echo '  make deploy-agents - Deploy SkyNet agents only'
	@echo ''
	@echo 'Monitoring:'
	@echo '  make agent-logs    - Show agent logs from both clusters'
	@echo '  make broker-info   - Show cluster registration on broker'
	@echo '  make frr-status    - Show FRR configurations'
	@echo '  make vtep-status   - Show VTEP resources'
	@echo ''
	@echo 'Development:'
	@echo '  make codegen       - Regenerate CRDs and deepcopy code'
	@echo '  make test          - Run unit tests'
	@echo '  make lint          - Run linters'
	@echo ''
	@echo 'All targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ── Quick Start (Test Environment) ───────────────────────────────────────────
# Simplified commands for SkyNet development and testing
# These use the test/ scripts for quick local development

.PHONY: clusters
clusters: test-setup ## Create 2 OVN-K KIND clusters with FRR-K8s

.PHONY: build-agent
build-agent: test-build-load ## Build and load SkyNet agent image to clusters

.PHONY: deploy-agents
deploy-agents: test-deploy ## Deploy SkyNet agents to both clusters (agents only)

.PHONY: deploy
deploy: ## Full deployment: create clusters, build, and deploy agents
	@$(MAKE) clusters
	@$(MAKE) build-agent
	@$(MAKE) deploy-agents
	@echo ""
	@echo "✓ Full deployment complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  make verify-bgp    - Verify BGP peering"
	@echo "  make agent-logs    - Show agent logs from both clusters"
	@echo "  make broker-info   - Show cluster registration on broker"
	@echo ""
	@echo "To run end-to-end tests:"
	@echo "  make e2e           - Run e2e test suite"

.PHONY: e2e
e2e: ## Run end-to-end tests (requires deployed clusters)
	@echo "Running SkyNet e2e tests..."
	@echo ""
	@echo "Test 1: Verify cluster registration"
	@$(MAKE) verify
	@echo ""
	@echo "Test 2: Verify BGP configuration"
	@$(MAKE) verify-bgp
	@echo ""
	@echo "✓ E2E tests complete!"

.PHONY: verify-bgp
verify-bgp: ## Verify BGP peering status
	cd test && ./verify-bgp.sh

.PHONY: verify
verify: test-verify ## Verify test setup (basic checks)

.PHONY: agent-logs
agent-logs: ## Show agent logs from both clusters
	@echo "=== Cluster1 Agent Logs ==="
	kubectl --context kind-cluster1 -n skynet-operator logs -l app=skynet-agent --tail=30 || true
	@echo ""
	@echo "=== Cluster2 Agent Logs ==="
	kubectl --context kind-cluster2 -n skynet-operator logs -l app=skynet-agent --tail=30 || true

.PHONY: broker-info
broker-info: ## Show cluster registration and status on broker
	@echo "=== Registered Clusters ==="
	kubectl --context kind-cluster1 -n skynet-broker get clusters -o wide
	@echo ""
	@echo "=== Cluster1 Details ==="
	kubectl --context kind-cluster1 -n skynet-broker get cluster cluster1 -o yaml || true
	@echo ""
	@echo "=== Cluster2 Details ==="
	kubectl --context kind-cluster1 -n skynet-broker get cluster cluster2 -o yaml || true

.PHONY: frr-status
frr-status: ## Show FRRConfiguration status in both clusters
	@echo "=== Cluster1 FRRConfigurations ==="
	kubectl --context kind-cluster1 get frrconfigurations -A || true
	@echo ""
	@echo "=== Cluster2 FRRConfigurations ==="
	kubectl --context kind-cluster2 get frrconfigurations -A || true

.PHONY: vtep-status
vtep-status: ## Show VTEP status in both clusters
	@echo "=== Cluster1 VTEPs ==="
	kubectl --context kind-cluster1 get vteps -o wide || true
	@echo ""
	@echo "=== Cluster2 VTEPs ==="
	kubectl --context kind-cluster2 get vteps -o wide || true

# Tear down everything
.PHONY: clean
clean: test-clean ## Clean up test environment

# ── Legacy All-in-one (Operator-based) ────────────────────────────────────────
# Original operator-based workflow (keeping for compatibility)
.PHONY: all-operator
all-operator: docker-build kind-create kind-load-only kind-deploy-all


OPERATOR_IMAGE = $(REPO):mcn-operator-$(VERSION)
BROKER_IMAGE   = $(REPO):mcn-broker-$(VERSION)
AGENT_IMAGE    = $(REPO):mcn-agent-$(VERSION)

# ── Build binaries locally ────────────────────────────────────────────────────

.PHONY: build
build: build-operator build-broker build-agent

.PHONY: build-operator
build-operator:
	go build -o bin/mcn-operator ./cmd/operator

.PHONY: build-broker
build-broker:
	go build -o bin/mcn-broker ./cmd/broker

.PHONY: build-agent
build-agent:
	go build -o bin/mcn-agent ./cmd/agent

# ── Build container images ────────────────────────────────────────────────────

.PHONY: docker-build
docker-build: docker-build-operator docker-build-broker docker-build-agent

.PHONY: docker-build-operator
docker-build-operator:
	docker build -f package/Dockerfile.operator -t $(OPERATOR_IMAGE) .

.PHONY: docker-build-broker
docker-build-broker:
	docker build -f package/Dockerfile.broker -t $(BROKER_IMAGE) .

.PHONY: docker-build-agent
docker-build-agent:
	docker build -f package/Dockerfile.agent -t $(AGENT_IMAGE) .

# ── Push container images ─────────────────────────────────────────────────────

.PHONY: docker-push
docker-push: docker-push-operator docker-push-broker docker-push-agent

.PHONY: docker-push-operator
docker-push-operator:
	docker push $(OPERATOR_IMAGE)

.PHONY: docker-push-broker
docker-push-broker:
	docker push $(BROKER_IMAGE)

.PHONY: docker-push-agent
docker-push-agent:
	docker push $(AGENT_IMAGE)

# ── KIND ──────────────────────────────────────────────────────────────────────
# Full from-scratch setup — run steps separately to avoid OOM from parallel builds:
#   Step 1:  make docker-build      (build images — needs Go + Docker on host)
#   Step 2:  make kind-setup        (create clusters + load images + deploy — all containerised)
#
# Tear down with: make clean
.PHONY: kind-setup
kind-setup: kind-create kind-load-only kind-deploy-all

# Build the setup container image (kind, kubectl, git, jq, jinjanate, docker CLI).
# Built once and reused by all KIND/deploy targets.
.PHONY: setup-image
setup-image:
	docker build -f package/Dockerfile.setup -t $(SETUP_IMAGE) .

# Create KIND clusters + install OVN-K IC CNI (via ovn-k contrib/kind.sh).
.PHONY: kind-create
kind-create: setup-image
	$(DOCKER_RUN) \
		-e OVN_K_REPO=$(OVN_K_REPO) \
		-e OVN_K_BRANCH=$(OVN_K_BRANCH) \
		-e OVN_K_DIR=$(OVN_K_DIR) \
		-e OVN_IMAGE=$(OVN_IMAGE) \
		-e WORKERS=$(WORKERS) \
		$(SETUP_IMAGE) \
		bash /workspace/scripts/kind/create-clusters.sh

# Load already-built images into KIND clusters (no docker-build).
.PHONY: kind-load-only
kind-load-only: setup-image
	$(DOCKER_RUN) \
		-e REPO=$(REPO) \
		-e VERSION=$(VERSION) \
		$(SETUP_IMAGE) \
		bash /workspace/scripts/kind/load-images.sh

# Build images AND load into KIND clusters in one step.
.PHONY: kind-load
kind-load: docker-build kind-load-only

# Deploy operator to all KIND clusters.
.PHONY: kind-deploy-all
kind-deploy-all: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		bash -c 'for c in $(CLUSTERS); do \
			kubectl apply -k /workspace/deploy/kind/ --kubeconfig /tmp/mcn-$$c.kubeconfig; \
		done'

# Delete KIND clusters and clean up kubeconfigs.
.PHONY: kind-clean
kind-clean: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		bash /workspace/scripts/kind/cleanup.sh

# ── Deploy (Operator-based) ──────────────────────────────────────────────────
# Deploy operator to any cluster (real or KIND) by kubeconfig or context.
# Usage: make deploy-operator CONTEXT=kind-cluster1
.PHONY: deploy-operator
deploy-operator: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		kubectl apply -k /workspace/deploy/ --context $(CONTEXT)

# Usage: make undeploy CONTEXT=kind-cluster1
.PHONY: undeploy
undeploy: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		kubectl delete -k /workspace/deploy/ --context $(CONTEXT) --ignore-not-found

# Deploy to a specific KIND cluster by name.
# Usage: make kind-deploy CLUSTER=cluster1
.PHONY: kind-deploy
kind-deploy: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		kubectl apply -k /workspace/deploy/kind/ --kubeconfig /tmp/mcn-$(CLUSTER).kubeconfig

# Undeploy from a specific KIND cluster.
# Usage: make kind-undeploy CLUSTER=cluster1
.PHONY: kind-undeploy
kind-undeploy: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		kubectl delete -k /workspace/deploy/kind/ --kubeconfig /tmp/mcn-$(CLUSTER).kubeconfig --ignore-not-found

# Generate deploy/install.yaml — single-file bundle for direct kubectl apply.
.PHONY: deploy-bundle
deploy-bundle: setup-image
	$(DOCKER_RUN) \
		$(SETUP_IMAGE) \
		bash -c 'kubectl kustomize /workspace/deploy/ > /workspace/deploy/install.yaml'

# ── Test & Development ───────────────────────────────────────────────────────

.PHONY: test
test: ## Run unit tests
	go test ./...

.PHONY: lint
lint: ## Run linters
	golangci-lint run ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

# ── Legacy Test Environment ──────────────────────────────────────────────────
# Original test-* targets (use new quick start targets above instead)

.PHONY: test-help
test-help: ## Show legacy test environment help (deprecated)
	@echo 'SkyNet Test Environment Commands:'
	@echo '  make test-setup         - Setup 2 KIND clusters with broker'
	@echo '  make test-build-load    - Build and load agent image'
	@echo '  make test-deploy        - Deploy agents to clusters'
	@echo '  make test-verify        - Verify the setup'
	@echo '  make test-mcn           - Create test MultiClusterNetwork'
	@echo '  make test-mcnc          - Create test MultiClusterNetworkConnect'
	@echo '  make test-all           - Run complete test setup'
	@echo '  make test-clean         - Clean up test environment'
	@echo ''
	@echo 'Monitoring Commands:'
	@echo '  make agent-logs-c1      - Show agent logs from cluster1'
	@echo '  make agent-logs-c2      - Show agent logs from cluster2'
	@echo '  make broker-clusters    - Show registered clusters'
	@echo '  make broker-mcns        - Show MultiClusterNetworks'
	@echo '  make vteps-c1           - Show VTEPs in cluster1'
	@echo '  make vteps-c2           - Show VTEPs in cluster2'

.PHONY: codegen
codegen: ## Generate deepcopy and CRD manifests
	./hack/update-codegen.sh

.PHONY: test-setup
test-setup: ## Setup KIND clusters with broker
	cd test && ./setup-clusters.sh

.PHONY: test-build-load
test-build-load: ## Build and load agent image to KIND clusters
	cd test && ./build-and-load.sh

.PHONY: test-deploy
test-deploy: ## Deploy agents to clusters
	cd test && ./deploy-agents.sh

.PHONY: test-verify
test-verify: ## Verify the setup
	cd test && ./verify-setup.sh

.PHONY: test-mcn
test-mcn: ## Create test MultiClusterNetwork
	cd test && ./create-mcn.sh

.PHONY: test-mcnc
test-mcnc: ## Create test MultiClusterNetworkConnect
	cd test && ./create-mcnc.sh test-mcn default

.PHONY: test-all
test-all: ## Run complete test setup
	cd test && ./run-all.sh

.PHONY: test-clean
test-clean: ## Clean up test environment
	cd test && ./cleanup.sh

.PHONY: agent-logs-c1
agent-logs-c1: ## Show agent logs from cluster1
	kubectl --context kind-cluster1 -n skynet-operator logs -l app=skynet-agent -f

.PHONY: agent-logs-c2
agent-logs-c2: ## Show agent logs from cluster2
	kubectl --context kind-cluster2 -n skynet-operator logs -l app=skynet-agent -f

.PHONY: broker-clusters
broker-clusters: ## Show registered clusters on broker
	kubectl --context kind-cluster1 -n skynet-broker get clusters -o wide

.PHONY: broker-mcns
broker-mcns: ## Show MultiClusterNetworks on broker
	kubectl --context kind-cluster1 -n skynet-broker get multiclusternetworks -o wide

.PHONY: vteps-c1
vteps-c1: ## Show VTEPs in cluster1
	kubectl --context kind-cluster1 get vteps -o wide

.PHONY: vteps-c2
vteps-c2: ## Show VTEPs in cluster2
	kubectl --context kind-cluster2 get vteps -o wide

.PHONY: frr-c1
frr-c1: ## Show FRRConfigurations in cluster1
	kubectl --context kind-cluster1 get frrconfigurations -o wide

.PHONY: frr-c2
frr-c2: ## Show FRRConfigurations in cluster2
	kubectl --context kind-cluster2 get frrconfigurations -o wide
