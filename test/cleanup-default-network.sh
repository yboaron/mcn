#!/bin/bash

# Cleanup default network test resources

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo "======================================================================"
log_info "Cleaning up default network test resources"
echo "======================================================================"

KUBECONFIG_C1="${SCRIPT_DIR}/kubeconfig-cluster1.yaml"
KUBECONFIG_C2="${SCRIPT_DIR}/kubeconfig-cluster2.yaml"

if [ ! -f "$KUBECONFIG_C1" ]; then
    kind export kubeconfig --name cluster1 --kubeconfig "$KUBECONFIG_C1" 2>/dev/null || true
fi

if [ ! -f "$KUBECONFIG_C2" ]; then
    kind export kubeconfig --name cluster2 --kubeconfig "$KUBECONFIG_C2" 2>/dev/null || true
fi

# Delete test pods
log_info "Deleting test pods..."
kubectl --kubeconfig="$KUBECONFIG_C1" delete pod test-pod-default-c1 --ignore-not-found > /dev/null 2>&1 || true
kubectl --kubeconfig="$KUBECONFIG_C2" delete pod test-pod-default-c2 --ignore-not-found > /dev/null 2>&1 || true

# Delete RouteAdvertisement CRs
log_info "Deleting RouteAdvertisement CRs..."
kubectl --kubeconfig="$KUBECONFIG_C1" delete routeadvertisements default-network-pod-routes --ignore-not-found > /dev/null 2>&1 || true
kubectl --kubeconfig="$KUBECONFIG_C2" delete routeadvertisements default-network-pod-routes --ignore-not-found > /dev/null 2>&1 || true

log_info "✓ Cleanup complete"
