#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
GREEN='\033[0;32m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

BROKER_CLUSTER="cluster1"
BROKER_NAMESPACE="skynet-broker"

log_info "Creating MultiClusterNetwork on broker..."

kubectl config use-context "kind-${BROKER_CLUSTER}"

# Create a Layer3 MultiClusterNetwork
cat <<EOF | kubectl apply -f -
apiVersion: skynet.io/v1
kind: MultiClusterNetwork
metadata:
  name: test-mcn
  namespace: ${BROKER_NAMESPACE}
spec:
  vni: 5000
  routeTarget: "65000:5000"
  topology: Layer3
EOF

log_info "MultiClusterNetwork 'test-mcn' created"

# Show the MCN
kubectl get multiclusternetwork -n "${BROKER_NAMESPACE}" test-mcn -o yaml
