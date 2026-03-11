#!/bin/bash

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

CLUSTER1_NAME="cluster1"
CLUSTER2_NAME="cluster2"
IMAGE_NAME="skynet-agent:latest"

log_info "=== Building and Loading SkyNet Agent Image ==="

# Build the image
log_info "Building docker image..."
cd "${PROJECT_ROOT}"
docker build -f test/Dockerfile -t "${IMAGE_NAME}" .

# Load to cluster1
log_info "Loading image to ${CLUSTER1_NAME}..."
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER1_NAME}$"; then
    kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER1_NAME}"
else
    log_warn "Cluster ${CLUSTER1_NAME} not found, skipping"
fi

# Load to cluster2
log_info "Loading image to ${CLUSTER2_NAME}..."
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER2_NAME}$"; then
    kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER2_NAME}"
else
    log_warn "Cluster ${CLUSTER2_NAME} not found, skipping"
fi

log_info "=== Build and load complete ==="
log_info ""
log_info "Next step: ./deploy-agents.sh"
