# Skynet — minimal Makefile for kind-based dev (2 clusters + broker + agents).
# When you build container images (package/Dockerfile.*), prefer tags like
# $(REPO):skynet-operator-$(VERSION), :skynet-broker-, :skynet-agent- (not mcn-*).

.DEFAULT_GOAL := help

REPO    ?= quay.io/aswinsuryan/skynet
VERSION ?= latest

.PHONY: help
help:
	@echo 'Skynet'
	@echo '  make deploy        — fresh 2× OVN-K kind + broker + FRR + skynet-agent + verify (fully automated)'
	@echo '  make clusters      — clusters + broker CRDs + pools ConfigMap + RBAC (test/setup-clusters-v2.sh)'
	@echo '  make clean         — delete kind clusters + test artifacts (test/cleanup.sh)'
	@echo '  make verify-bgp    — after deploy: agents, broker Cluster CRs, VTEP, FRR, BGP mesh checks'
	@echo ''
	@echo 'Default Network Tests:'
	@echo '  make test-default-network  — test default network pod connectivity across clusters via RouteAdvertisement'
	@echo '  make cleanup-default       — cleanup default network test resources'
	@echo ''
	@echo 'CUDN Stretching Tests:'
	@echo '  make test-cudn-l3  — test Layer3 CUDN stretching across clusters'
	@echo '  make cleanup-cudn  — cleanup CUDN test resources'
	@echo ''
	@echo 'Pieces (also used by deploy):'
	@echo '  make build-agent   — docker build + kind load (test/build-and-load.sh)'
	@echo '  make deploy-agents — kubectl apply test/manifests/skynet-agent + Secret/ConfigMap (test/deploy-agents.sh)'
	@echo '  make fix-bgp       — patch FRR-K8s to enable BGP on 0.0.0.0:179 (workaround for upstream)'
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

.PHONY: fix-bgp
fix-bgp:
	cd test && ./workaround-frr-bgp.sh

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
	$(MAKE) fix-bgp
	@echo ""
	@echo "⏳ Waiting 60s for BGP sessions to establish..."
	@sleep 60
	@echo ""
	@echo "Running BGP verification..."
	$(MAKE) verify-bgp

.PHONY: verify-bgp
verify-bgp:
	cd test && ./verify-bgp.sh

.PHONY: test
test:
	go test ./...

.PHONY: test-cudn-l3
test-cudn-l3:
	@echo ""
	@echo "Running comprehensive CUDN Layer3 stretching test..."
	@echo "This will:"
	@echo "  1. Create MCNC on cluster1 (SkyNet creates CUDN with EVPN + deploy pod)"
	@echo "  2. Create MCNC on cluster2 (SkyNet creates CUDN with EVPN + deploy pod)"
	@echo "  3. Verify SkyNet agent configuration (EVPN transport, VNI, RT)"
	@echo "  4. Test cross-cluster pod-to-pod connectivity via CUDN"
	@echo ""
	cd test && ./test-cudn-l3.sh

.PHONY: cleanup-cudn
cleanup-cudn:
	cd test && ./cleanup-cudn-test.sh

.PHONY: test-default-network
test-default-network:
	@echo ""
	@echo "Running default network connectivity test..."
	@echo "This will:"
	@echo "  1. Verify non-overlapping pod CIDRs"
	@echo "  2. Apply RouteAdvertisement CR to both clusters"
	@echo "  3. Create test pods on default network"
	@echo "  4. Verify BGP route propagation"
	@echo "  5. Test cross-cluster pod-to-pod connectivity"
	@echo ""
	cd test && ./test-default-network.sh

.PHONY: cleanup-default
cleanup-default:
	cd test && ./cleanup-default-network.sh
