#!/bin/bash
# Workaround for FRR-K8s BGP Listening Limitation
#
# ISSUE: FRR-K8s starts bgpd with "-A 127.0.0.1" which prevents BGP from
#        accepting connections on the node's external IP.
#
# ROOT CAUSE: FRR-K8s hardcodes bgpd startup options in the frr-k8s-frr-startup
#             ConfigMap with no API to override them.
#
# WORKAROUND: Manually patch the ConfigMap to use "-A 0.0.0.0" (listen on all
#             interfaces) and restart FRR pods.
#
# UPSTREAM TRACKING:
#   - FRR-K8s issue: TBD (to be filed)
#   - Long-term fix: Add API to FRRConfiguration to control daemon options
#
# REFERENCES:
#   - Similar workaround in POC:
#     https://github.com/yboaron/ovn-bgp-mcn-udn-poc/blob/main/scripts/2-configure-bgp.sh#L50

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Patch FRR startup ConfigMap to enable BGP on external interfaces
patch_frr_startup() {
    local context=$1
    local cluster=$2

    log_info "Patching FRR startup ConfigMap on ${cluster}..."

    # Get current ConfigMap
    kubectl --context "$context" get configmap -n frr-k8s-system frr-k8s-frr-startup -o yaml > /tmp/frr-startup-${cluster}.yaml

    # Patch bgpd_options to listen on 0.0.0.0 instead of 127.0.0.1
    sed -i 's/bgpd_options="   -A 127.0.0.1 -p 0 --limit-fds 100000"/bgpd_options="   -A 0.0.0.0 -p 179 --limit-fds 100000"/' \
        /tmp/frr-startup-${cluster}.yaml

    # Apply patched ConfigMap
    kubectl --context "$context" apply -f /tmp/frr-startup-${cluster}.yaml

    # Restart FRR pods to apply changes
    log_info "Restarting FRR pods on ${cluster}..."
    kubectl --context "$context" rollout restart daemonset/frr-k8s-daemon -n frr-k8s-system

    log_info "✓ ${cluster} FRR ConfigMap patched"
}

# Main
main() {
    echo ""
    log_warn "╔═══════════════════════════════════════════════════════════════════╗"
    log_warn "║  FRR-K8s BGP Listening Workaround                                ║"
    log_warn "╚═══════════════════════════════════════════════════════════════════╝"
    echo ""
    log_warn "This script patches FRR-K8s to enable BGP peering on external IPs."
    log_warn "This is a WORKAROUND for a known FRR-K8s limitation."
    echo ""

    # Patch both clusters
    patch_frr_startup "kind-cluster1" "cluster1"
    patch_frr_startup "kind-cluster2" "cluster2"

    echo ""
    log_info "Waiting for FRR pods to be ready..."
    kubectl --context kind-cluster1 rollout status daemonset/frr-k8s-daemon -n frr-k8s-system --timeout=120s
    kubectl --context kind-cluster2 rollout status daemonset/frr-k8s-daemon -n frr-k8s-system --timeout=120s

    echo ""
    log_info "✓ Workaround applied successfully"
    echo ""
    log_info "Next steps:"
    log_info "  - Wait 30 seconds for agent reconciliation"
    log_info "  - Run: make verify-bgp"
    echo ""
}

main "$@"
