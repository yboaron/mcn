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

# ── All-in-one ────────────────────────────────────────────────────────────────
# Single command to do everything in the correct order:
#   1. build images  2. create clusters  3. load images  4. deploy
# Tear down with: make clean
.PHONY: all
all: docker-build kind-create kind-load-only kind-deploy-all

# Tear down everything
.PHONY: clean
clean: kind-clean


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

# ── Deploy ────────────────────────────────────────────────────────────────────
# Deploy to any cluster (real or KIND) by kubeconfig or context.
# Usage: make deploy CONTEXT=kind-cluster1
.PHONY: deploy
deploy: setup-image
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

# ── Test ──────────────────────────────────────────────────────────────────────

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run ./...

# ── Tidy ─────────────────────────────────────────────────────────────────────

.PHONY: tidy
tidy:
	go mod tidy

# ── SkyNet Test Environment ──────────────────────────────────────────────────
# Simple local testing with KIND clusters (alternative to the full kind-* targets above)

.PHONY: test-help
test-help: ## Show test environment help
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
	kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent -f

.PHONY: agent-logs-c2
agent-logs-c2: ## Show agent logs from cluster2
	kubectl --context kind-cluster2 -n skynet-system logs -l app=skynet-agent -f

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
