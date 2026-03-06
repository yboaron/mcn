#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Copyright Contributors to the Submariner project.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Shared utility functions for MCN KIND scripts.

# Colours
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # no colour

log_info()    { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# cluster_exists <cluster-name>
# Returns 0 if the KIND cluster already exists, 1 otherwise.
cluster_exists() {
    local name="$1"
    kind get clusters 2>/dev/null | grep -qx "${name}"
}

# load_kubeconfig <cluster-name>
# Exports the kubeconfig for the given KIND cluster and sets KUBECONFIG.
# Stores the kubeconfig under /tmp/mcn-<cluster-name>.kubeconfig.
load_kubeconfig() {
    local name="$1"
    local kubeconfig="/tmp/mcn-${name}.kubeconfig"

    kind export kubeconfig --name "${name}" --kubeconfig "${kubeconfig}" 2>/dev/null
    export KUBECONFIG="${kubeconfig}"
    log_info "Kubeconfig for ${name} exported to ${kubeconfig}"
}

# wait_for_nodes <kubeconfig>
# Waits until all nodes in the cluster report Ready.
wait_for_nodes() {
    local kubeconfig="$1"

    log_info "Waiting for all nodes to be Ready (OVN-K may take a few minutes)..."
    kubectl --kubeconfig "${kubeconfig}" wait node \
        --all \
        --for=condition=Ready \
        --timeout=300s
    log_info "All nodes are Ready."
}

# print_cluster_summary <cluster-name> <kubeconfig>
# Prints node names and IPs for the cluster.
print_cluster_summary() {
    local name="$1"
    local kubeconfig="$2"

    log_info "=== Cluster: ${name} ==="
    kubectl --kubeconfig "${kubeconfig}" get nodes -o wide --no-headers \
        | awk '{printf "  %-35s %-15s %s\n", $1, $6, $5}'
}
