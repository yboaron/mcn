#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_section() {
    echo -e "${BLUE}=== $1 ===${NC}"
}

CLUSTER1_NAME="cluster1"
CLUSTER2_NAME="cluster2"
BROKER_CLUSTER="cluster1"
BROKER_NAMESPACE="skynet-broker"

# Check cluster connectivity
check_cluster() {
    local cluster_name=$1
    log_section "Checking ${cluster_name}"

    kubectl config use-context "kind-${cluster_name}"

    # Check nodes
    log_info "Nodes in ${cluster_name}:"
    kubectl get nodes -o wide

    # Check agent pod
    log_info "SkyNet agent status:"
    kubectl -n skynet-operator get pods -l app=skynet-agent

    # Check agent logs (last 20 lines)
    log_info "Recent agent logs:"
    kubectl -n skynet-operator logs -l app=skynet-agent --tail=20 || log_warn "No logs available yet"

    echo ""
}

# Check broker resources
check_broker() {
    log_section "Checking Broker Resources"

    kubectl config use-context "kind-${BROKER_CLUSTER}"

    # Check broker namespace
    log_info "Broker namespace:"
    kubectl get namespace "${BROKER_NAMESPACE}" || log_error "Broker namespace not found"

    # Check CRDs
    log_info "SkyNet CRDs:"
    kubectl get crds | grep skynet.io || log_warn "No SkyNet CRDs found"

    # Check Cluster CRs
    log_info "Cluster resources on broker:"
    kubectl -n "${BROKER_NAMESPACE}" get clusters -o wide || log_warn "No clusters registered yet"

    # Show cluster details
    if kubectl -n "${BROKER_NAMESPACE}" get clusters &>/dev/null; then
        for cluster in $(kubectl -n "${BROKER_NAMESPACE}" get clusters -o name); do
            log_info "Details for ${cluster}:"
            kubectl -n "${BROKER_NAMESPACE}" get "${cluster}" -o yaml | grep -A 20 "spec:\|status:"
            echo ""
        done
    fi

    # Check MultiClusterNetworks
    log_info "MultiClusterNetworks:"
    kubectl -n "${BROKER_NAMESPACE}" get multiclusternetworks -o wide || log_warn "No MCNs found"

    echo ""
}

# Check local cluster resources
check_local_resources() {
    local cluster_name=$1
    log_section "Checking Local Resources in ${cluster_name}"

    kubectl config use-context "kind-${cluster_name}"

    # Check VTEPs
    log_info "VTEP resources:"
    kubectl get vteps -o wide || log_warn "No VTEPs found (or CRD not installed)"

    # Check FRRConfigurations
    log_info "FRRConfiguration resources:"
    kubectl get frrconfigurations -o wide || log_warn "No FRRConfigurations found (or CRD not installed)"

    # Check MultiClusterNetworkConnect
    log_info "MultiClusterNetworkConnect resources:"
    kubectl get mcnc --all-namespaces || log_warn "No MCNCs found"

    # Check UserDefinedNetworks
    log_info "UserDefinedNetwork resources:"
    kubectl get udn --all-namespaces || log_warn "No UDNs found (or CRD not installed)"

    echo ""
}

# Check network connectivity test
check_connectivity() {
    log_section "Network Connectivity Check"

    log_warn "Cross-cluster connectivity test requires test pods"
    log_info "To test connectivity:"
    log_info "1. Deploy test pods in both clusters in the connected namespace"
    log_info "2. Exec into a pod and ping the other cluster's pod IP"

    echo ""
}

# Main verification
main() {
    log_section "SkyNet Setup Verification"
    echo ""

    check_cluster "${CLUSTER1_NAME}"
    check_cluster "${CLUSTER2_NAME}"
    check_broker
    check_local_resources "${CLUSTER1_NAME}"
    check_local_resources "${CLUSTER2_NAME}"
    check_connectivity

    log_section "Verification Complete"
    log_info ""
    log_info "Useful commands:"
    log_info "  Watch agent logs: kubectl --context kind-cluster1 -n skynet-operator logs -l app=skynet-agent -f"
    log_info "  Check clusters: kubectl --context kind-cluster1 -n ${BROKER_NAMESPACE} get clusters"
    log_info "  Check MCNs: kubectl --context kind-cluster1 -n ${BROKER_NAMESPACE} get multiclusternetworks"
    log_info "  Check VTEPs: kubectl --context kind-cluster1 get vteps"
}

main "$@"
