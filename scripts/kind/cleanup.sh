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

# Deletes MCN KIND clusters and cleans up exported kubeconfigs.
#
# Usage:
#   ./scripts/kind/cleanup.sh
#   CLUSTERS="cluster1" ./scripts/kind/cleanup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=scripts/lib/utils.sh
source "${SCRIPT_DIR}/../lib/utils.sh"

CLUSTERS="${CLUSTERS:-cluster1 cluster2}"

### Main ###

log_info "MCN KIND cleanup"
log_info "Clusters to delete: ${CLUSTERS}"

for cluster in ${CLUSTERS}; do
    if cluster_exists "${cluster}"; then
        log_info "Deleting cluster '${cluster}'..."
        kind delete cluster --name "${cluster}"
        log_info "Cluster '${cluster}' deleted."
    else
        log_warn "Cluster '${cluster}' does not exist — skipping."
    fi

    # Clean up exported kubeconfig (may be root-owned if written from container)
    local_kubeconfig="/tmp/mcn-${cluster}.kubeconfig"
    if [[ -f "${local_kubeconfig}" ]]; then
        rm -f "${local_kubeconfig}" 2>/dev/null || \
            log_warn "Could not remove ${local_kubeconfig} (permission denied — run: sudo rm -f ${local_kubeconfig})"
    fi
done

log_info "Cleanup complete."
