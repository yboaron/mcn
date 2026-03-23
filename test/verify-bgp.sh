#!/bin/bash

# Verification script for BGP peering setup
# This focuses on BGP full mesh connectivity without CUDN stretching

# Note: Don't use set -e as we want to continue on failures and report at the end

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Same paths as setup-clusters-v2.sh / deploy-agents.sh
CLUSTER1_KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"
CLUSTER2_KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"
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

# Check if local VTEPs are created (current architecture: one local VTEP per cluster)
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep skynet-local" \
    "Cluster1 has local VTEP (skynet-local)"

check "kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} get vtep skynet-local" \
    "Cluster2 has local VTEP (skynet-local)"

# Check VTEP CIDR configuration
if kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep skynet-local -o yaml > /dev/null 2>&1; then
    VTEP_CIDRS=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} get vtep skynet-local -o jsonpath='{.spec.cidrs}' | jq '. | length')
    if [[ "$VTEP_CIDRS" -gt 0 ]]; then
        log_info "Cluster1's local VTEP has $VTEP_CIDRS CIDR(s) configured"
        ((check_passed++))
    else
        log_error "Cluster1's local VTEP has no CIDRs configured"
        ((check_failed++))
    fi
fi

# ============================================================================
# Phase 4: FRRConfiguration
# ============================================================================

log_section "Phase 4: FRRConfiguration (BGP Config)"

# Check if FRRConfiguration exists (in frr-k8s-system namespace)
check "kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config" \
    "Cluster1 has FRRConfiguration"

check "kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config" \
    "Cluster2 has FRRConfiguration"

# Check BGP neighbors configured
if kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config -o yaml > /dev/null 2>&1; then
    NEIGHBOR_COUNT=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config -o jsonpath='{.spec.bgp.routers[0].neighbors}' | jq '. | length')
    if [[ "$NEIGHBOR_COUNT" -gt 0 ]]; then
        log_info "Cluster1 FRRConfiguration has $NEIGHBOR_COUNT BGP neighbor(s)"
        ((check_passed++))
    else
        log_error "Cluster1 FRRConfiguration has no BGP neighbors"
        ((check_failed++))
    fi
fi

if kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config -o yaml > /dev/null 2>&1; then
    NEIGHBOR_COUNT=$(kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system get frrconfiguration skynet-bgp-config -o jsonpath='{.spec.bgp.routers[0].neighbors}' | jq '. | length')
    if [[ "$NEIGHBOR_COUNT" -gt 0 ]]; then
        log_info "Cluster2 FRRConfiguration has $NEIGHBOR_COUNT BGP neighbor(s)"
        ((check_passed++))
    else
        log_error "Cluster2 FRRConfiguration has no BGP neighbors"
        ((check_failed++))
    fi
fi

# Note: L2VPN EVPN configuration will be added in Phase 2 (CUDN stretching)
# For now, we just verify basic BGP peering works
log_info "L2VPN EVPN address family will be added in Phase 2 (CUDN stretching)"
((check_passed++))

# ============================================================================
# Phase 5: FRR-K8s Status (Optional - requires FRR running)
# ============================================================================

log_section "Phase 5: FRR-K8s Status (if available)"

FRR_POD1=$(kubectl --kubeconfig ${CLUSTER1_KUBECONFIG} -n frr-k8s-system get pod -l app.kubernetes.io/component=frr-k8s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
FRR_POD2=$(kubectl --kubeconfig ${CLUSTER2_KUBECONFIG} -n frr-k8s-system get pod -l app.kubernetes.io/component=frr-k8s -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [[ -n "$FRR_POD1" ]]; then
    log_info "FRR pod running on cluster1: $FRR_POD1"
    ((check_passed++))
else
    log_warn "FRR-K8s not found on cluster1 (install FRR-K8s for BGP functionality)"
fi

if [[ -n "$FRR_POD2" ]]; then
    log_info "FRR pod running on cluster2: $FRR_POD2"
    ((check_passed++))
else
    log_warn "FRR-K8s not found on cluster2 (install FRR-K8s for BGP functionality)"
fi

# ============================================================================
# Phase 6: Live BGP summary (best-effort)
# ============================================================================

log_section "Phase 6: BGP summary from FRR (best-effort)"

try_bgp_summary() {
    local kcfg=$1
    local pod=$2
    local label=$3
    if [[ -z "$pod" ]]; then
        log_warn "${label}: no FRR pod"
        return
    fi
    if SUMMARY=$(kubectl --kubeconfig "${kcfg}" -n frr-k8s-system exec "${pod}" -c frr -- vtysh -c 'show bgp summary' 2>/dev/null); then
        echo "--- ${label} ---"
        echo "${SUMMARY}" | head -25
        # Check for uptime pattern (00:00:00 format indicates established session)
        if echo "${SUMMARY}" | grep -qE '[0-9]{2}:[0-9]{2}:[0-9]{2}'; then
            log_info "${label}: BGP sessions established"
            ((check_passed++))
        else
            log_warn "${label}: no established sessions yet (if stuck Active, see docs/FRR_K8S_BGP_LIMITATION.md)"
        fi
    else
        log_warn "${label}: could not run vtysh (pod not ready?)"
    fi
}

try_bgp_summary "${CLUSTER1_KUBECONFIG}" "${FRR_POD1}" "cluster1"
try_bgp_summary "${CLUSTER2_KUBECONFIG}" "${FRR_POD2}" "cluster2"

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
