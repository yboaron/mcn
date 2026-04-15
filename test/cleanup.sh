#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

CLUSTER1_NAME="cluster1"
CLUSTER2_NAME="cluster2"

log_info "=== Cleaning up SkyNet test environment ==="

# Delete KIND clusters
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER1_NAME}$"; then
    log_info "Deleting cluster ${CLUSTER1_NAME}..."
    kind delete cluster --name "${CLUSTER1_NAME}"
else
    log_warn "Cluster ${CLUSTER1_NAME} not found"
fi

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER2_NAME}$"; then
    log_info "Deleting cluster ${CLUSTER2_NAME}..."
    kind delete cluster --name "${CLUSTER2_NAME}"
else
    log_warn "Cluster ${CLUSTER2_NAME} not found"
fi

# Clean up generated files
log_info "Cleaning up generated files..."
rm -f "${SCRIPT_DIR}/broker-token-"*.txt
rm -f "${SCRIPT_DIR}/broker-server.txt"
rm -rf "${PROJECT_ROOT}/output"

# Remove docker image
log_info "Removing docker image..."
docker rmi skynet-agent:latest 2>/dev/null || log_warn "Image skynet-agent:latest not found"

# Clean up OVN-K test containers (frr, bgpserver)
log_info "Removing OVN-K test containers..."
docker rm -f frr bgpserver 2>/dev/null || log_warn "OVN-K test containers not found"

# Clean up Docker networks (kind, underlay, bgpnet)
log_info "Removing Docker networks..."
docker network rm kind underlay bgpnet 2>/dev/null || log_warn "Some networks not found"

log_info "=== Cleanup complete ==="
