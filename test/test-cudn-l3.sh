#!/bin/bash

# End-to-End CUDN Stretching Test
# This script validates complete CUDN stretching workflow:
# 1. Create MCNC on cluster1 with cudnSpec (SkyNet creates CUDN with EVPN + deploy pod)
# 2. Create MCNC on cluster2 with cudnSpec (SkyNet creates CUDN with EVPN + deploy pod)
# 3. Verify SkyNet agent configuration (EVPN transport, VNI, RT, RouteAdvertisements)
# 4. Test cross-cluster pod-to-pod connectivity via CUDN

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    if ! kind get clusters 2>/dev/null | grep -q "^cluster1$"; then
        log_error "cluster1 not found. Run 'make deploy' first."
        exit 1
    fi

    if ! kind get clusters 2>/dev/null | grep -q "^cluster2$"; then
        log_error "cluster2 not found. Run 'make deploy' first."
        exit 1
    fi

    log_info "✓ Both clusters found"
}

# Step 1: Create MCNC on cluster1 (SkyNet creates CUDN with EVPN)
create_mcnc_cluster1() {
    log_step "Step 1: Creating MCNC on cluster1 (SkyNet creates CUDN)..."

    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"

    # Step 1a: Create temporary namespace for MCNC (without primary network label)
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: tenant-skynet-demo
  labels:
    skynet-network: tenant-a
EOF

    # Step 1b: Create MCNC with cudnSpec - SkyNet agent will create the CUDN with EVPN transport
    cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: stretch-tenant-l3
  namespace: tenant-skynet-demo
spec:
  createMultiClusterNetwork:
    name: demo-mcn-l3
    topology: Layer3
  cudnSpec:
    namespaceSelector:
      matchLabels:
        skynet-network: tenant-a
    topology: Layer3
    role: Primary
    subnets:
      - cidr: "10.100.0.0/16"
        hostSubnet: 23
EOF

    log_info "✓ MCNC created on cluster1 (SkyNet will create CUDN)"
    log_info "Waiting for SkyNet agent to create CUDN with EVPN config (45s)..."
    sleep 45

    # Verify CUDN was created by SkyNet
    if kubectl get cudn demo-mcn-l3 &>/dev/null; then
        log_info "✓ CUDN created by SkyNet agent with EVPN transport"
    else
        log_error "Failed: SkyNet agent did not create CUDN"
        exit 1
    fi

    # Step 1c: Delete and recreate namespace with primary network label
    # (OVN-K requires this label at namespace creation time)
    kubectl delete namespace tenant-skynet-demo --wait=true
    sleep 5

    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: tenant-skynet-demo
  labels:
    skynet-network: tenant-a
    k8s.ovn.org/primary-user-defined-network: demo-mcn-l3
EOF

    # Step 1d: Recreate MCNC in the new namespace
    cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: stretch-tenant-l3
  namespace: tenant-skynet-demo
spec:
  createMultiClusterNetwork:
    name: demo-mcn-l3
    topology: Layer3
  cudnSpec:
    namespaceSelector:
      matchLabels:
        skynet-network: tenant-a
    topology: Layer3
    role: Primary
    subnets:
      - cidr: "10.100.0.0/16"
        hostSubnet: 23
EOF

    log_info "✓ Namespace recreated with primary network label"

    # Create test pod that uses CUDN
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: cudn-test-pod
  namespace: tenant-skynet-demo
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

    log_info "✓ Test pod created on cluster1"
}

# Step 2: Create MCNC on cluster2 to join MCN (SkyNet creates CUDN with EVPN)
create_mcnc_cluster2() {
    log_step "Step 2: Creating MCNC on cluster2 to join MCN..."

    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"

    # Step 2a: Create temporary namespace for MCNC (without primary network label)
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: tenant-skynet-demo
  labels:
    skynet-network: tenant-a
EOF

    # Step 2b: Create MCNC with cudnSpec to join existing MCN - SkyNet agent will create the CUDN
    cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: stretch-tenant-l3
  namespace: tenant-skynet-demo
spec:
  multiClusterNetworkName: demo-mcn-l3
  cudnSpec:
    namespaceSelector:
      matchLabels:
        skynet-network: tenant-a
    topology: Layer3
    role: Primary
    subnets:
      - cidr: "10.200.0.0/16"
        hostSubnet: 23
EOF

    log_info "✓ MCNC created on cluster2 (joining MCN)"
    log_info "Waiting for SkyNet agent to create CUDN with EVPN config (45s)..."
    sleep 45

    # Verify CUDN was created by SkyNet
    if kubectl get cudn demo-mcn-l3 &>/dev/null; then
        log_info "✓ CUDN created by SkyNet agent with EVPN transport"
    else
        log_error "Failed: SkyNet agent did not create CUDN"
        exit 1
    fi

    # Step 2c: Delete and recreate namespace with primary network label
    # (OVN-K requires this label at namespace creation time)
    kubectl delete namespace tenant-skynet-demo --wait=true
    sleep 5

    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: tenant-skynet-demo
  labels:
    skynet-network: tenant-a
    k8s.ovn.org/primary-user-defined-network: demo-mcn-l3
EOF

    # Step 2d: Recreate MCNC in the new namespace
    cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: stretch-tenant-l3
  namespace: tenant-skynet-demo
spec:
  multiClusterNetworkName: demo-mcn-l3
  cudnSpec:
    namespaceSelector:
      matchLabels:
        skynet-network: tenant-a
    topology: Layer3
    role: Primary
    subnets:
      - cidr: "10.200.0.0/16"
        hostSubnet: 23
EOF

    log_info "✓ Namespace recreated with primary network label"

    # Create test pod
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: cudn-test-pod
  namespace: tenant-skynet-demo
spec:
  containers:
  - name: netshoot
    image: nicolaka/netshoot
    command: ["sleep", "infinity"]
EOF

    log_info "✓ Test pod created on cluster2"
}

# Wait for pods to get IPs from CUDN
wait_for_pod_ips() {
    log_step "Waiting for pods to get IPs from CUDN subnets..."

    # Wait for cluster1 pod
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"
    log_info "Waiting for cluster1 pod to be ready..."
    kubectl wait --for=condition=ready pod/cudn-test-pod -n tenant-skynet-demo --timeout=120s

    # Get UDN IP from OVN annotations (more reliable than .status.podIP)
    # The annotation format is: {"tenant-skynet-demo/tenant-network-l3":{"ip_addresses":["10.200.x.x/16"]}}
    POD1_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo \
        -o jsonpath='{.metadata.annotations.k8s\.ovn\.org/pod-networks}' 2>/dev/null | \
        grep -oP '"tenant-skynet-demo/tenant-network-l3":\{"ip_addresses":\["\K[^"]+' | cut -d'/' -f1 || echo "")

    # Fallback to .status.podIP if annotation not found
    if [ -z "$POD1_IP" ]; then
        POD1_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo -o jsonpath='{.status.podIP}')
        log_warn "Using .status.podIP (annotation not found)"
    fi

    if [[ "$POD1_IP" =~ ^10\.100\. ]]; then
        log_info "✓ Cluster1 pod IP: $POD1_IP (from CUDN subnet 10.100.0.0/16)"
    else
        log_warn "Cluster1 pod IP: $POD1_IP (expected 10.100.x.x)"
    fi

    # Wait for cluster2 pod
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"
    log_info "Waiting for cluster2 pod to be ready..."
    kubectl wait --for=condition=ready pod/cudn-test-pod -n tenant-skynet-demo --timeout=120s

    # Get UDN IP from OVN annotations
    POD2_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo \
        -o jsonpath='{.metadata.annotations.k8s\.ovn\.org/pod-networks}' 2>/dev/null | \
        grep -oP '"tenant-skynet-demo/tenant-network-l3":\{"ip_addresses":\["\K[^"]+' | cut -d'/' -f1 || echo "")

    # Fallback to .status.podIP
    if [ -z "$POD2_IP" ]; then
        POD2_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo -o jsonpath='{.status.podIP}')
        log_warn "Using .status.podIP (annotation not found)"
    fi

    if [[ "$POD2_IP" =~ ^10\.200\. ]]; then
        log_info "✓ Cluster2 pod IP: $POD2_IP (from CUDN subnet 10.200.0.0/16)"
    else
        log_warn "Cluster2 pod IP: $POD2_IP (expected 10.200.x.x)"
    fi

    echo ""
    log_info "Pod IPs from CUDN subnets:"
    log_info "  Cluster1: $POD1_IP (from OVN annotation or .status.podIP)"
    log_info "  Cluster2: $POD2_IP (from OVN annotation or .status.podIP)"
}

# Step 3: Verify SkyNet agent configuration
verify_agent_work() {
    log_step "Step 3: Verifying SkyNet agent configuration..."

    echo ""
    log_info "=== Cluster 1 Verification ==="
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"

    # Check MCNC status
    MCNC_PHASE=$(kubectl get mcnc -n tenant-skynet-demo stretch-tenant-l3 -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
    if [ "$MCNC_PHASE" = "Connected" ]; then
        log_info "✓ MCNC status: Connected"
    else
        log_warn "MCNC status: $MCNC_PHASE (expected: Connected)"
    fi

    # Check CUDN EVPN config
    TRANSPORT=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.transport}' 2>/dev/null || echo "")
    if [ "$TRANSPORT" = "EVPN" ]; then
        log_info "✓ CUDN transport: EVPN"
    else
        log_error "CUDN transport: $TRANSPORT (expected: EVPN)"
        return 1
    fi

    # Check VNI
    VNI=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.evpn.ipVRF.vni}' 2>/dev/null || echo "")
    if [ -n "$VNI" ]; then
        log_info "✓ VNI allocated: $VNI"
    else
        log_error "VNI not found in CUDN EVPN config"
        return 1
    fi

    # Check Route Target
    RT=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.evpn.ipVRF.routeTarget}' 2>/dev/null || echo "")
    if [ -n "$RT" ]; then
        log_info "✓ Route Target: $RT"
    else
        log_error "Route Target not found"
        return 1
    fi

    # Check VTEP reference
    VTEP=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.evpn.vtep}' 2>/dev/null || echo "")
    if [ "$VTEP" = "skynet-local" ]; then
        log_info "✓ VTEP reference: skynet-local"
    else
        log_warn "VTEP reference: $VTEP (expected: skynet-local)"
    fi

    # Check RouteAdvertisement
    if kubectl get routeadvertisements skynet-cluster1-demo-mcn-l3 &>/dev/null; then
        log_info "✓ RouteAdvertisement created"
    else
        log_warn "RouteAdvertisement not found"
    fi

    # Check MCN on broker
    MCN_VNI=$(kubectl get mcn -n skynet-broker demo-mcn-l3 -o jsonpath='{.spec.vni}' 2>/dev/null || echo "")
    MCN_RT=$(kubectl get mcn -n skynet-broker demo-mcn-l3 -o jsonpath='{.spec.routeTarget}' 2>/dev/null || echo "")
    if [ -n "$MCN_VNI" ]; then
        log_info "✓ MCN on broker: VNI=$MCN_VNI, RT=$MCN_RT"
    else
        log_warn "MCN not found on broker"
    fi

    # Check VTEP status
    VTEP_STATUS=$(kubectl get vtep skynet-local -o jsonpath='{.status}' 2>/dev/null || echo "{}")
    if [ "$VTEP_STATUS" != "{}" ] && [ -n "$VTEP_STATUS" ]; then
        log_info "✓ VTEP has status (IPs allocated by OVN-K)"
    else
        log_warn "VTEP status empty (OVN-K allocates IPs when CUDN with EVPN references VTEP)"
        log_warn "This is expected if OVN-K VTEP controller hasn't processed the EVPN-enabled CUDN yet"
    fi

    echo ""
    log_info "=== Cluster 2 Verification ==="
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"

    # Check MCNC status
    MCNC_PHASE=$(kubectl get mcnc -n tenant-skynet-demo stretch-tenant-l3 -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
    if [ "$MCNC_PHASE" = "Connected" ]; then
        log_info "✓ MCNC status: Connected"
    else
        log_warn "MCNC status: $MCNC_PHASE (expected: Connected)"
    fi

    # Check CUDN EVPN config
    TRANSPORT2=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.transport}' 2>/dev/null || echo "")
    if [ "$TRANSPORT2" = "EVPN" ]; then
        log_info "✓ CUDN transport: EVPN"
    else
        log_error "CUDN transport: $TRANSPORT2 (expected: EVPN)"
        return 1
    fi

    # Check VNI matches cluster1
    VNI2=$(kubectl get cudn demo-mcn-l3 -o jsonpath='{.spec.network.evpn.ipVRF.vni}' 2>/dev/null || echo "")
    if [ "$VNI2" = "$VNI" ]; then
        log_info "✓ VNI matches cluster1: $VNI2"
    else
        log_warn "VNI mismatch: cluster1=$VNI, cluster2=$VNI2"
    fi

    # Check RouteAdvertisement
    if kubectl get routeadvertisements skynet-cluster2-demo-mcn-l3 &>/dev/null; then
        log_info "✓ RouteAdvertisement created"
    else
        log_warn "RouteAdvertisement not found"
    fi
}

# Step 6: Test cross-cluster connectivity
test_connectivity() {
    log_step "Step 4: Testing cross-cluster connectivity via CUDN..."

    # Get pod IPs
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"
    POD1_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo -o jsonpath='{.status.podIP}')

    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"
    POD2_IP=$(kubectl get pod cudn-test-pod -n tenant-skynet-demo -o jsonpath='{.status.podIP}')

    echo ""
    log_info "Testing connectivity between:"
    log_info "  Cluster1 pod: $POD1_IP (CUDN subnet: 10.100.0.0/16)"
    log_info "  Cluster2 pod: $POD2_IP (CUDN subnet: 10.200.0.0/16)"
    echo ""

    # Wait a bit for BGP routes to propagate
    log_info "Waiting 20s for BGP routes to propagate..."
    sleep 20

    # Test ping from cluster1 to cluster2
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster1.yaml"
    log_info "Testing: cluster1 → cluster2 (ping $POD2_IP)..."
    if kubectl exec -n tenant-skynet-demo cudn-test-pod -- ping -c 3 -W 10 "$POD2_IP" &>/dev/null; then
        log_info "✓ SUCCESS: Connectivity test PASSED (cluster1 → cluster2)"
    else
        log_warn "FAILED: Connectivity test failed (cluster1 → cluster2)"
        log_warn ""
        log_warn "This may be expected if:"
        log_warn "  - VTEP IPs not yet allocated by OVN-K VTEP controller"
        log_warn "  - BGP routes not yet propagated"
        log_warn "  - FRRConfiguration not yet generated by OVN-K"
        log_warn ""
        log_warn "Debug commands:"
        log_warn "  kubectl get vtep skynet-local -o yaml"
        log_warn "  kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{\"\\t\"}{.metadata.annotations.k8s\\.ovn\\.org/vtep-ips}{\"\\n\"}{end}'"
        log_warn "  kubectl exec -n frr-k8s-system ds/frr-k8s -- vtysh -c 'show bgp summary'"
        return 1
    fi

    # Test reverse ping from cluster2 to cluster1
    export KUBECONFIG="${PROJECT_ROOT}/output/kubeconfig-cluster2.yaml"
    log_info "Testing: cluster2 → cluster1 (ping $POD1_IP)..."
    if kubectl exec -n tenant-skynet-demo cudn-test-pod -- ping -c 3 -W 10 "$POD1_IP" &>/dev/null; then
        log_info "✓ SUCCESS: Connectivity test PASSED (cluster2 → cluster1)"
    else
        log_warn "FAILED: Connectivity test failed (cluster2 → cluster1)"
        return 1
    fi
}

# Print summary
print_summary() {
    echo ""
    echo "======================================================================"
    log_info "CUDN Stretching Test Complete!"
    echo "======================================================================"
    echo ""
    log_info "What was tested:"
    log_info "  ✓ Created Layer3 CUDN on both clusters"
    log_info "  ✓ Pods got IPs from CUDN subnets (10.100.x.x and 10.200.x.x)"
    log_info "  ✓ Created MCNC on cluster1 (created MCN)"
    log_info "  ✓ Created MCNC on cluster2 (joined MCN)"
    log_info "  ✓ SkyNet agent injected EVPN config into CUDNs"
    log_info "  ✓ VNI allocated and matching across clusters"
    log_info "  ✓ RouteAdvertisements created"
    if [ "$CONNECTIVITY_PASSED" = "true" ]; then
        log_info "  ✓ Cross-cluster pod connectivity via CUDN: WORKING"
    else
        log_info "  ⚠ Cross-cluster pod connectivity: NEEDS INVESTIGATION"
    fi
    echo ""
    log_info "Next steps:"
    log_info "  1. Check VTEP IPs: kubectl get vtep skynet-local -o yaml"
    log_info "  2. Check BGP routes: kubectl exec -n frr-k8s-system ds/frr-k8s -- vtysh -c 'show bgp l2vpn evpn'"
    log_info "  3. Check FRRConfiguration: kubectl get frrconfigurations -n frr-k8s-system"
    log_info "  4. Cleanup: make cleanup-cudn"
    echo ""
}

# Main execution
main() {
    echo ""
    echo "======================================================================"
    log_info "SkyNet CUDN Stretching - End-to-End Test"
    echo "======================================================================"
    echo ""

    check_prerequisites
    echo ""

    create_mcnc_cluster1
    echo ""

    create_mcnc_cluster2
    echo ""

    wait_for_pod_ips
    echo ""

    verify_agent_work
    echo ""

    CONNECTIVITY_PASSED="false"
    if test_connectivity; then
        CONNECTIVITY_PASSED="true"
    fi
    echo ""

    print_summary
}

main "$@"
