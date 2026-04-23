#!/bin/bash

# Cleanup CUDN stretching test resources

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_info "=== Cleaning up CUDN test resources ==="

# Cleanup cluster1
log_info "Cleaning up cluster1..."
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"

# Delete namespaces (cascade deletes MCNC, pods, etc.)
kubectl delete namespace tenant-skynet-demo --ignore-not-found=true
kubectl delete namespace tenant-skynet-demo-l2 --ignore-not-found=true

# Delete cluster-scoped CUDNs
kubectl delete cudn tenant-network-l3 --ignore-not-found=true
kubectl delete cudn tenant-network-l2 --ignore-not-found=true
kubectl delete cudn demo-mcn-l3 --ignore-not-found=true

# Cleanup cluster2
log_info "Cleaning up cluster2..."
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"

# Delete namespaces (cascade deletes MCNC, pods, etc.)
kubectl delete namespace tenant-skynet-demo --ignore-not-found=true
kubectl delete namespace tenant-skynet-demo-l2 --ignore-not-found=true

# Delete cluster-scoped CUDNs
kubectl delete cudn tenant-network-l3 --ignore-not-found=true
kubectl delete cudn tenant-network-l2 --ignore-not-found=true
kubectl delete cudn demo-mcn-l3 --ignore-not-found=true

# Wait for cleanup
log_info "Waiting for cleanup to complete..."
sleep 10

# Check broker for leftover MCNs
log_info "Checking broker for MCN cleanup..."
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"

MCN_COUNT=$(kubectl get mcn -n skynet-broker --no-headers 2>/dev/null | wc -l)
if [ "$MCN_COUNT" -gt 0 ]; then
    log_warn "Found $MCN_COUNT MCNs on broker (should be auto-deleted when last cluster leaves)"
    kubectl get mcn -n skynet-broker
else
    log_info "✓ All MCNs cleaned up from broker"
fi

log_info "=== Cleanup complete ==="
