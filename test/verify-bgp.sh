#!/bin/bash

# Verification script for BGP peering setup
# This focuses on BGP full mesh connectivity without CUDN stretching

# Note: Don't use set -e as we want to continue on failures and report at the end

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
KUBECONFIG_DIR="${PROJECT_ROOT}/output/kubeconfigs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

CLUSTER1_KUBECONFIG="${KUBECONFIG_DIR}/kind-config-cluster1"
CLUSTER2_KUBECONFIG="${KUBECONFIG_DIR}/kind-config-cluster2"
BROKER_NS="skynet-broker"
AGENT_NS="skynet-operator"

log_info() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[!]${NC} $1"
}

log_section() {
    echo -e "\n${BLUE}===${NC} $1 ${BLUE}===${NC}"
}

check_passed=0
check_failed=0

check() {
    if eval "$1" > /dev/null 2>&1; then
        log_info "$2"
        ((check_passed++))
        return 0
    else
        log_error "$2"
        ((check_failed++))
        return 1
    fi
}

# ============================================================================
# Phase 1: Agent Status
# ============================================================================

log_section "Phase 1: Agent Status"

# Check if agent pods are running
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${AGENT_NS} get pod -l app=skynet-agent -o jsonpath='{.items[0].status.phase}' | grep -q Running" \
    "Cluster1 agent is Running"

check "kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n ${AGENT_NS} get pod -l app=skynet-agent -o jsonpath='{.items[0].status.phase}' | grep -q Running" \
    "Cluster2 agent is Running"

# Get agent pod names for later use
AGENT1_POD=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${AGENT_NS} get pod -l app=skynet-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
AGENT2_POD=$(kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n ${AGENT_NS} get pod -l app=skynet-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [[ -n "$AGENT1_POD" ]]; then
    log_info "Cluster1 agent pod: $AGENT1_POD"
fi
if [[ -n "$AGENT2_POD" ]]; then
    log_info "Cluster2 agent pod: $AGENT2_POD"
fi

# ============================================================================
# Phase 2: Cluster Registration
# ============================================================================

log_section "Phase 2: Cluster Registration on Broker"

# Check if clusters are registered
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster1" \
    "Cluster1 registered on broker"

check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster2" \
    "Cluster2 registered on broker"

# Check ASN allocation
CLUSTER1_ASN=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster1 -o jsonpath='{.spec.asn}' 2>/dev/null)
CLUSTER2_ASN=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster2 -o jsonpath='{.spec.asn}' 2>/dev/null)

if [[ -n "$CLUSTER1_ASN" ]] && [[ -n "$CLUSTER2_ASN" ]] && [[ "$CLUSTER1_ASN" != "$CLUSTER2_ASN" ]]; then
    log_info "ASN allocation: cluster1=$CLUSTER1_ASN, cluster2=$CLUSTER2_ASN (unique ✓)"
    ((check_passed++))
else
    log_error "ASN allocation failed or not unique"
    ((check_failed++))
fi

# Check VTEP CIDR allocation
CLUSTER1_VTEP=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster1 -o jsonpath='{.spec.vtepCIDR}' 2>/dev/null)
CLUSTER2_VTEP=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster2 -o jsonpath='{.spec.vtepCIDR}' 2>/dev/null)

if [[ -n "$CLUSTER1_VTEP" ]] && [[ -n "$CLUSTER2_VTEP" ]] && [[ "$CLUSTER1_VTEP" != "$CLUSTER2_VTEP" ]]; then
    log_info "VTEP CIDR allocation: cluster1=$CLUSTER1_VTEP, cluster2=$CLUSTER2_VTEP (unique ✓)"
    ((check_passed++))
else
    log_error "VTEP CIDR allocation failed or not unique"
    ((check_failed++))
fi

# Check endpoints reported
CLUSTER1_ENDPOINTS=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster1 -o jsonpath='{.status.endpoints}' 2>/dev/null)
CLUSTER2_ENDPOINTS=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster2 -o jsonpath='{.status.endpoints}' 2>/dev/null)

if [[ -n "$CLUSTER1_ENDPOINTS" ]] && [[ "$CLUSTER1_ENDPOINTS" != "[]" ]]; then
    CLUSTER1_ENDPOINT_COUNT=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster1 -o jsonpath='{.status.endpoints}' | jq '. | length')
    log_info "Cluster1 has $CLUSTER1_ENDPOINT_COUNT node endpoint(s) reported"
    ((check_passed++))
else
    log_error "Cluster1 has no endpoints reported"
    ((check_failed++))
fi

if [[ -n "$CLUSTER2_ENDPOINTS" ]] && [[ "$CLUSTER2_ENDPOINTS" != "[]" ]]; then
    CLUSTER2_ENDPOINT_COUNT=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get cluster cluster2 -o jsonpath='{.status.endpoints}' | jq '. | length')
    log_info "Cluster2 has $CLUSTER2_ENDPOINT_COUNT node endpoint(s) reported"
    ((check_passed++))
else
    log_error "Cluster2 has no endpoints reported"
    ((check_failed++))
fi

# ============================================================================
# Phase 3: VTEP Resources
# ============================================================================

log_section "Phase 3: VTEP Resources"

# Check if VTEPs are created for remote clusters
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep cluster2" \
    "Cluster1 has VTEP for cluster2"

check "kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} get vtep cluster1" \
    "Cluster2 has VTEP for cluster1"

# Check VTEP endpoints
if kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep cluster2 -o yaml > /dev/null 2>&1; then
    VTEP_ENDPOINTS=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep cluster2 -o jsonpath='{.spec.endpoints}' | jq '. | length')
    if [[ "$VTEP_ENDPOINTS" -gt 0 ]]; then
        log_info "Cluster1's VTEP for cluster2 has $VTEP_ENDPOINTS endpoint(s)"
        ((check_passed++))
    else
        log_error "Cluster1's VTEP for cluster2 has no endpoints"
        ((check_failed++))
    fi
fi

# ============================================================================
# Phase 4: FRRConfiguration
# ============================================================================

log_section "Phase 4: FRRConfiguration (BGP Config)"

# Check if FRRConfiguration exists
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get frrconfiguration skynet-bgp-config" \
    "Cluster1 has FRRConfiguration"

check "kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} get frrconfiguration skynet-bgp-config" \
    "Cluster2 has FRRConfiguration"

# Check BGP neighbors configured
if kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get frrconfiguration skynet-bgp-config -o yaml > /dev/null 2>&1; then
    NEIGHBOR_COUNT=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get frrconfiguration skynet-bgp-config -o jsonpath='{.spec.bgp.routers[0].neighbors}' | jq '. | length')
    if [[ "$NEIGHBOR_COUNT" -gt 0 ]]; then
        log_info "Cluster1 FRRConfiguration has $NEIGHBOR_COUNT BGP neighbor(s)"
        ((check_passed++))
    else
        log_error "Cluster1 FRRConfiguration has no BGP neighbors"
        ((check_failed++))
    fi
fi

if kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} get frrconfiguration skynet-bgp-config -o yaml > /dev/null 2>&1; then
    NEIGHBOR_COUNT=$(kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} get frrconfiguration skynet-bgp-config -o jsonpath='{.spec.bgp.routers[0].neighbors}' | jq '. | length')
    if [[ "$NEIGHBOR_COUNT" -gt 0 ]]; then
        log_info "Cluster2 FRRConfiguration has $NEIGHBOR_COUNT BGP neighbor(s)"
        ((check_passed++))
    else
        log_error "Cluster2 FRRConfiguration has no BGP neighbors"
        ((check_failed++))
    fi
fi

# Check address family configuration (should be l2vpn/evpn)
if kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get frrconfiguration skynet-bgp-config -o yaml 2>/dev/null | grep -q "l2vpn"; then
    log_info "Cluster1 FRRConfiguration has L2VPN EVPN address family"
    ((check_passed++))
else
    log_error "Cluster1 FRRConfiguration missing L2VPN EVPN address family"
    ((check_failed++))
fi

# ============================================================================
# Phase 5: FRR-K8s Status (Optional - requires FRR running)
# ============================================================================

log_section "Phase 5: FRR-K8s Status (if available)"

FRR_POD1=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system get pod -l app.kubernetes.io/name=frr-k8s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
FRR_POD2=$(kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system get pod -l app.kubernetes.io/name=frr-k8s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [[ -n "$FRR_POD1" ]]; then
    log_info "FRR pod running on cluster1: $FRR_POD1"

    # Try to get BGP summary
    log_warn "To check BGP sessions on cluster1, run:"
    echo "  kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system exec -it $FRR_POD1 -- vtysh -c 'show bgp summary'"
else
    log_warn "FRR-K8s not found on cluster1 (install FRR-K8s for BGP functionality)"
fi

if [[ -n "$FRR_POD2" ]]; then
    log_info "FRR pod running on cluster2: $FRR_POD2"

    log_warn "To check BGP sessions on cluster2, run:"
    echo "  kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system exec -it $FRR_POD2 -- vtysh -c 'show bgp summary'"
else
    log_warn "FRR-K8s not found on cluster2 (install FRR-K8s for BGP functionality)"
fi

# ============================================================================
# Summary
# ============================================================================

log_section "Verification Summary"

echo ""
echo "Total checks: $((check_passed + check_failed))"
echo -e "  ${GREEN}Passed: $check_passed${NC}"
echo -e "  ${RED}Failed: $check_failed${NC}"
echo ""

if [[ $check_failed -eq 0 ]]; then
    log_info "All checks passed! ✓"
    echo ""
    echo "Next steps:"
    echo "1. Check BGP sessions in FRR (see commands above)"
    echo "2. Verify EVPN routes: kubectl exec <frr-pod> -- vtysh -c 'show bgp l2vpn evpn'"
    echo "3. Once BGP is working, proceed with CUDN stretching tests"
    exit 0
else
    log_error "Some checks failed. Review the output above."
    echo ""
    echo "Troubleshooting tips:"
    echo "1. Check agent logs: kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${AGENT_NS} logs -l app=skynet-agent"
    echo "2. Check broker resources: kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n ${BROKER_NS} get clusters -o yaml"
    echo "3. Restart agents if needed: kubectl rollout restart deployment/skynet-agent -n ${AGENT_NS}"
    exit 1
fi
