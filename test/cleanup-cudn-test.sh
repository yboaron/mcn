#!/bin/bash

# Cleanup CUDN stretching test resources
# Note: Continue cleanup even if some resources are not found or fail to delete

set +e  # Don't exit on error - we want to clean up as much as possible

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

# Delete MCNC - this is what a real user would do
# Agent handles: RouteAdvertisement deletion, MCN finalizer removal
# Note: Agent does NOT delete CUDN in localCUDN pattern (user owns it)
log_info "Deleting MCNC stretch-tenant-l3..."
kubectl delete mcnc -n tenant-skynet-demo stretch-tenant-l3 --ignore-not-found=true --timeout=30s || log_warn "Failed to delete MCNC"

# Wait for agent cleanup to complete (RA deletion)
log_info "Waiting for agent to clean up RouteAdvertisement..."
sleep 5

# Delete namespace first (cascade deletes pods, allows CUDN to clean up)
log_info "Deleting namespace tenant-skynet-demo (cascade deletes pods)..."
kubectl delete namespace tenant-skynet-demo --ignore-not-found=true --wait=false 2>/dev/null || true

# Delete user-created CUDN (localCUDN pattern - user owns it)
# Note: Namespace deletion may block until CUDN is deleted (chicken-egg)
log_info "Deleting user-created CUDN demo-mcn-l3..."
kubectl delete cudn demo-mcn-l3 --ignore-not-found=true --timeout=30s || log_warn "CUDN deletion timed out (may have finalizer)"

# Wait for namespace deletion to complete
log_info "Waiting for namespace deletion to complete..."
kubectl wait --for=delete namespace/tenant-skynet-demo --timeout=30s 2>/dev/null || log_warn "Namespace still exists"

# Cleanup for other test variations (if they exist)
kubectl delete cudn tenant-network-l3 --ignore-not-found=true 2>/dev/null || true
kubectl delete cudn tenant-network-l2 --ignore-not-found=true 2>/dev/null || true
kubectl delete namespace tenant-skynet-demo-l2 --ignore-not-found=true 2>/dev/null || true

# Cleanup cluster2
log_info "Cleaning up cluster2..."
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"

# Delete MCNC - this is what a real user would do
# Agent handles: RouteAdvertisement deletion, MCN finalizer removal
# Note: Agent does NOT delete CUDN in localCUDN pattern (user owns it)
log_info "Deleting MCNC stretch-tenant-l3..."
kubectl delete mcnc -n tenant-skynet-demo stretch-tenant-l3 --ignore-not-found=true --timeout=30s || log_warn "Failed to delete MCNC"

# Wait for agent cleanup to complete (RA deletion)
log_info "Waiting for agent to clean up RouteAdvertisement..."
sleep 5

# Delete namespace first (cascade deletes pods, allows CUDN to clean up)
log_info "Deleting namespace tenant-skynet-demo (cascade deletes pods)..."
kubectl delete namespace tenant-skynet-demo --ignore-not-found=true --wait=false 2>/dev/null || true

# Delete user-created CUDN (localCUDN pattern - user owns it)
# Note: Namespace deletion may block until CUDN is deleted (chicken-egg)
log_info "Deleting user-created CUDN demo-mcn-l3..."
kubectl delete cudn demo-mcn-l3 --ignore-not-found=true --timeout=30s || log_warn "CUDN deletion timed out (may have finalizer)"

# Wait for namespace deletion to complete
log_info "Waiting for namespace deletion to complete..."
kubectl wait --for=delete namespace/tenant-skynet-demo --timeout=30s 2>/dev/null || log_warn "Namespace still exists"

# Cleanup for other test variations (if they exist)
kubectl delete cudn tenant-network-l3 --ignore-not-found=true 2>/dev/null || true
kubectl delete cudn tenant-network-l2 --ignore-not-found=true 2>/dev/null || true
kubectl delete namespace tenant-skynet-demo-l2 --ignore-not-found=true 2>/dev/null || true

# Wait for agent finalizers to complete
log_info "Waiting for agent finalizers to complete (RA deletion, MCN finalizer removal)..."
sleep 15

# Verify agent cleanup worked correctly
log_info "Verifying agent cleanup..."
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"

CLEANUP_SUCCESS=true

# Check cluster1 for orphaned RouteAdvertisements
RA_COUNT=$(kubectl get routeadvertisements -l skynet.io/managed-by=skynet-agent --no-headers 2>/dev/null | wc -l)
if [ "$RA_COUNT" -gt 0 ]; then
    log_warn "Cluster1: Found $RA_COUNT orphaned RouteAdvertisements (agent cleanup failed)"
    kubectl get routeadvertisements -l skynet.io/managed-by=skynet-agent
    CLEANUP_SUCCESS=false
else
    log_info "✓ Cluster1: All RouteAdvertisements cleaned up by agent"
fi

# Check cluster2 for orphaned RouteAdvertisements
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"
RA_COUNT=$(kubectl get routeadvertisements -l skynet.io/managed-by=skynet-agent --no-headers 2>/dev/null | wc -l)
if [ "$RA_COUNT" -gt 0 ]; then
    log_warn "Cluster2: Found $RA_COUNT orphaned RouteAdvertisements (agent cleanup failed)"
    kubectl get routeadvertisements -l skynet.io/managed-by=skynet-agent
    CLEANUP_SUCCESS=false
else
    log_info "✓ Cluster2: All RouteAdvertisements cleaned up by agent"
fi

# Check broker for MCN cleanup (should be auto-deleted when both clusters removed finalizers)
export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"
MCN_COUNT=$(kubectl get multiclusternetworks -n skynet-broker --no-headers 2>/dev/null | wc -l)
if [ "$MCN_COUNT" -gt 0 ]; then
    log_warn "Broker: Found $MCN_COUNT MCNs (checking finalizers...)"
    kubectl get multiclusternetworks -n skynet-broker -o custom-columns=NAME:.metadata.name,FINALIZERS:.metadata.finalizers
    log_warn "MCNs should auto-delete when last cluster removes its finalizer"
    CLEANUP_SUCCESS=false
else
    log_info "✓ Broker: All MCNs cleaned up (finalizers worked correctly)"
fi

if [ "$CLEANUP_SUCCESS" = false ]; then
    echo ""
    log_warn "Agent cleanup did not complete successfully - manual intervention may be needed"
    log_warn "This indicates a bug in the agent's cleanup/finalizer logic"
fi

log_info "=== Cleanup complete ==="
