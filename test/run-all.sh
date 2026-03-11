#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_section() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}=== $1${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Prompt user
prompt_continue() {
    local message=$1
    echo ""
    read -p "$(echo -e ${GREEN}${message}${NC} [y/N]: )" -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Skipping..."
        return 1
    fi
    return 0
}

log_section "SkyNet Complete Setup"

# Check prerequisites first
"${SCRIPT_DIR}/check-prereqs.sh" || exit 1

echo ""
log_info "This script will:"
log_info "1. Setup 2 KIND clusters with OVN-Kubernetes + FRR-K8s"
log_info "2. Build the SkyNet agent container"
log_info "3. Deploy agents to both clusters"
log_info "4. Create a test MultiClusterNetwork"
log_info "5. Connect namespaces to the MCN"
log_info "6. Verify the setup"
log_info ""
log_info "This will take approximately 15-20 minutes"

if ! prompt_continue "Continue with full setup?"; then
    exit 0
fi

# Step 1: Setup clusters
log_section "Step 1: Setup Clusters"
"${SCRIPT_DIR}/setup-clusters.sh"

# Step 2: Build and load image
log_section "Step 2: Build and Load Agent Image"
"${SCRIPT_DIR}/build-and-load.sh"

# Wait a bit for clusters to stabilize
log_info "Waiting 10 seconds for clusters to stabilize..."
sleep 10

# Step 3: Deploy agents
log_section "Step 3: Deploy Agents"
"${SCRIPT_DIR}/deploy-agents.sh"

# Wait for agents to start
log_info "Waiting 20 seconds for agents to start..."
sleep 20

# Step 4: Initial verification
log_section "Step 4: Initial Verification"
"${SCRIPT_DIR}/verify-setup.sh"

# Ask if user wants to create MCN
if prompt_continue "Create test MultiClusterNetwork?"; then
    log_section "Step 5: Create MultiClusterNetwork"
    "${SCRIPT_DIR}/create-mcn.sh"

    # Wait a bit for MCN to sync
    log_info "Waiting 5 seconds for MCN to sync..."
    sleep 5
fi

# Ask if user wants to create MCNC
if prompt_continue "Create MultiClusterNetworkConnect in default namespace?"; then
    log_section "Step 6: Create MultiClusterNetworkConnect"
    "${SCRIPT_DIR}/create-mcnc.sh" test-mcn default

    # Wait for MCNC reconciliation
    log_info "Waiting 10 seconds for MCNC reconciliation..."
    sleep 10
fi

# Final verification
log_section "Final Verification"
"${SCRIPT_DIR}/verify-setup.sh"

log_section "Setup Complete!"

log_info ""
log_info "Clusters are ready!"
log_info ""
log_info "Useful commands:"
log_info "  Watch agent logs:"
log_info "    kubectl --context kind-cluster1 -n skynet-system logs -l app=skynet-agent -f"
log_info ""
log_info "  Check clusters on broker:"
log_info "    kubectl --context kind-cluster1 -n skynet-broker get clusters -o wide"
log_info ""
log_info "  Check VTEPs:"
log_info "    kubectl --context kind-cluster1 get vteps"
log_info "    kubectl --context kind-cluster2 get vteps"
log_info ""
log_info "  Cleanup:"
log_info "    ./cleanup.sh"
