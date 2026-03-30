# Skynet — minimal Makefile for kind-based dev (2 clusters + broker + agents).
# When you build container images (package/Dockerfile.*), prefer tags like
# $(REPO):skynet-operator-$(VERSION), :skynet-broker-, :skynet-agent- (not mcn-*).

.DEFAULT_GOAL := help

REPO    ?= quay.io/aswinsuryan/skynet
VERSION ?= latest

.PHONY: help
help:
	@echo 'Skynet'
	@echo '  make deploy        — fresh 2× OVN-K kind + broker + FRR + skynet-agent (manifest apply)'
	@echo '  make clusters      — clusters + broker CRDs + pools ConfigMap + RBAC (test/setup-clusters-v2.sh)'
	@echo '  make clean         — delete kind clusters + test artifacts (test/cleanup.sh)'
	@echo '  make verify-bgp    — after deploy: agents, broker Cluster CRs, VTEP, FRR, BGP mesh checks'
	@echo ''
	@echo 'Pieces (also used by deploy):'
	@echo '  make build-agent   — docker build + kind load (test/build-and-load.sh)'
	@echo '  make deploy-agents — kubectl apply test/manifests/skynet-agent + Secret/ConfigMap (test/deploy-agents.sh)'
	@echo ''
	@echo '  make test          — go test ./...'
	@echo ''
	@echo 'Image tag hint: $(REPO):skynet-operator-$(VERSION) (operator), same REPO for broker/agent.'

.PHONY: clean
clean:
	cd test && ./cleanup.sh

.PHONY: clusters
clusters:
	cd test && ./setup-clusters-v2.sh

.PHONY: build-agent
build-agent:
	cd test && ./build-and-load.sh

.PHONY: deploy-agents
deploy-agents:
	cd test && ./deploy-agents.sh

.PHONY: deploy
deploy:
	@echo ""
	@echo "⚠️  Fresh deploy: removes existing kind clusters. Ctrl+C within 5s to abort."
	@sleep 5
	$(MAKE) clean
	$(MAKE) clusters
	$(MAKE) build-agent
	$(MAKE) deploy-agents
	@echo ""
	@echo "Applying FRR-K8s BGP listening port workaround..."
	cd test && ./workaround-frr-bgp.sh
	@echo ""
	@echo "✓ deploy complete. Next: make verify-bgp"

.PHONY: verify-bgp
verify-bgp:
	cd test && ./verify-bgp.sh

.PHONY: test
test:
	go test ./...
