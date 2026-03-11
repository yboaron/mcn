#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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
MCN_NAME="${1:-test-mcn}"
NAMESPACE="${2:-default}"

# Create MCNC in cluster1
log_info "Creating MultiClusterNetworkConnect in ${CLUSTER1_NAME}/${NAMESPACE}..."
kubectl config use-context "kind-${CLUSTER1_NAME}"

# Create namespace if it doesn't exist
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: ${MCN_NAME}-connect
  namespace: ${NAMESPACE}
spec:
  localCUDN: default
  multiClusterNetworkName: ${MCN_NAME}
EOF

log_info "MCNC created in ${CLUSTER1_NAME}/${NAMESPACE}"

# Create MCNC in cluster2
log_info "Creating MultiClusterNetworkConnect in ${CLUSTER2_NAME}/${NAMESPACE}..."
kubectl config use-context "kind-${CLUSTER2_NAME}"

# Create namespace if it doesn't exist
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetworkConnect
metadata:
  name: ${MCN_NAME}-connect
  namespace: ${NAMESPACE}
spec:
  localCUDN: default
  multiClusterNetworkName: ${MCN_NAME}
EOF

log_info "MCNC created in ${CLUSTER2_NAME}/${NAMESPACE}"

log_info ""
log_info "=== MultiClusterNetworkConnect created in both clusters ==="
log_info ""
log_info "Check MCNC status:"
log_info "  kubectl --context kind-${CLUSTER1_NAME} -n ${NAMESPACE} get mcnc"
log_info "  kubectl --context kind-${CLUSTER2_NAME} -n ${NAMESPACE} get mcnc"
log_info ""
log_info "Check UserDefinedNetwork:"
log_info "  kubectl --context kind-${CLUSTER1_NAME} -n ${NAMESPACE} get udn"
log_info "  kubectl --context kind-${CLUSTER2_NAME} -n ${NAMESPACE} get udn"
