# SkyNet Code Structure and Deployment Flow

This document walks through the SkyNet codebase organization and the complete `make deploy` workflow.

---

## Table of Contents

1. [Code Structure Overview](#code-structure-overview)
2. [Package Details](#package-details)
3. [Deployment Flow: make deploy](#deployment-flow-make-deploy)
4. [Data Flow: Agent Lifecycle](#data-flow-agent-lifecycle)

---

## Code Structure Overview

```
skynet/
├── cmd/                          # Executable entry points
│   ├── agent/                    # SkyNet Agent binary
│   ├── operator/                 # SkyNet Operator binary (allocates ASN/VTEP)
│   └── broker/                   # Broker stub (not needed - broker is pure storage)
│
├── pkg/                          # Core implementation packages
│   ├── agent/                    # Agent logic (all the intelligence)
│   │   ├── allocator/            # VNI allocation
│   │   ├── bgp/                  # BGP/FRR configuration generation
│   │   ├── bootstrap/            # Environment config loading
│   │   ├── controller/           # MCNC (MultiClusterNetworkConnect) controller
│   │   ├── endpoint/             # Node endpoint discovery and reporting
│   │   ├── mcn/                  # MultiClusterNetwork lifecycle management
│   │   ├── syncer/               # Broker sync using Submariner Admiral
│   │   └── vtep/                 # VTEP resource management
│   │
│   ├── operator/                 # Operator logic (resource allocation)
│   │   ├── alloc/                # ASN and VTEP CIDR allocation
│   │   └── skynet/               # Skynet CR reconciliation
│   │
│   ├── apis/skynet.io/v1/       # CRD definitions
│   └── names/                    # Common naming constants
│
├── test/                         # Deployment and test scripts
│   ├── manifests/                # Kubernetes manifests
│   │   └── skynet-agent/         # Agent deployment YAML
│   ├── setup-clusters-v2.sh      # Create KIND clusters with OVN-K + FRR-K8s
│   ├── build-and-load.sh         # Build agent image and load to clusters
│   ├── deploy-agents.sh          # Deploy agents to clusters
│   ├── verify-bgp.sh             # Verify BGP peering status
│   ├── workaround-frr-bgp.sh     # Apply FRR-K8s listening fix
│   └── cleanup.sh                # Clean up test environment
│
├── deploy/crds/                  # CRD YAML files
├── Makefile                      # Main build/deploy automation
└── Dockerfile.agent              # Agent container image build
```

---

## Package Details

### 1. cmd/agent/main.go

**Entry point for SkyNet Agent**

- Reads configuration from environment (`pkg/agent/bootstrap/env.go`; only klog verbosity uses flags)
- Initializes Kubernetes clients (local cluster + broker)
- Creates and starts the Agent instance
- Runs reconciliation loop

**Key responsibilities:**
- Bootstrap configuration from env vars/config
- Create local and broker K8s clients
- Start agent reconciliation loop (30s interval)

---

### 2. pkg/agent/agent.go

**Main Agent Orchestrator - The Brain**

**Reconciliation Loop (every 30 seconds):**

```
1. Cluster Registration
   ├─ Read ASN/VTEP from broker Cluster CR (operator already allocated)
   ├─ Register cluster if not exists
   └─ Update cluster spec with local metadata

2. Endpoint Discovery
   ├─ List all K8s nodes
   ├─ Extract node IPs (BGP peer IPs)
   ├─ Allocate VTEP IPs from cluster's VTEP CIDR
   └─ Report endpoints to broker (Cluster CR status)

3. VTEP Management
   ├─ Ensure local VTEP exists (skynet-local)
   └─ Configure VTEP with cluster's VTEP CIDR

4. BGP Configuration
   ├─ Fetch remote clusters from broker
   ├─ Build FRRConfiguration with:
   │  ├─ Local ASN
   │  ├─ Remote cluster neighbors (full mesh)
   │  └─ eBGP multihop settings
   └─ Apply/Update FRRConfiguration

5. Heartbeat
   └─ Update cluster status.lastHeartbeatTime
```

**Key methods:**
- `Start()`: Main reconciliation loop
- `reconcileClusterState()`: Core orchestration
- `ensureClusterRegistered()`: Register on broker
- `reconcileEndpoints()`: Node discovery
- `reconcileBGP()`: BGP config generation

---

### 3. pkg/agent/syncer/broker_syncer.go

**Broker Synchronization using Submariner Admiral**

- Syncs Cluster CRs from broker to local cluster
- Syncs MultiClusterNetwork CRs from broker to local
- Uses Submariner's `syncer.ResourceSyncerConfig`
- Bidirectional sync: local changes → broker, broker changes → local

**Why Submariner Admiral?**
- Battle-tested for multi-cluster sync
- Handles conflict resolution
- Automatic retry and reconciliation
- Used in production by Submariner project

---

### 4. pkg/agent/endpoint/endpoint_reporter.go

**Node Endpoint Discovery**

**Flow:**
1. List all nodes in local cluster
2. For each node:
   - Extract InternalIP (used as BGP peer IP)
   - Allocate VTEP IP from cluster's VTEP CIDR
   - Determine if node is route reflector (label-based)
3. Build Endpoint list
4. Report to broker via Cluster CR status.endpoints

**Endpoint structure:**
```go
type Endpoint struct {
    NodeName        string  // cluster1-worker
    BgpPeerIP      string  // 172.18.0.2 (node's InternalIP)
    VtepIP         string  // 100.0.0.188 (allocated from CIDR)
    RouteReflector bool    // For RR topology support
}
```

---

### 5. pkg/agent/bgp/bgp_configurator.go

**FRRConfiguration Generation**

**Creates BGP configuration for full mesh peering:**

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: skynet-bgp-config
  namespace: frr-k8s-system
spec:
  bgp:
    routers:
    - asn: 64512                    # Local cluster ASN (from operator)
      neighbors:
      - address: 172.18.0.5         # Remote cluster node 1
        asn: 64513                  # Remote cluster ASN
        ebgpMultiHop: true
      - address: 172.18.0.6         # Remote cluster node 2
        asn: 64513
        ebgpMultiHop: true
      - address: 172.18.0.7         # Remote cluster node 3
        asn: 64513
        ebgpMultiHop: true
```

**Key methods:**
- `ReconcileBGPConfig()`: Create/update FRRConfiguration
- `buildNeighbors()`: Build neighbor list from remote cluster endpoints
- Supports full mesh and route reflector topologies

---

### 6. pkg/agent/vtep/vtep_manager.go

**VTEP Resource Management**

**Creates local VTEP resource:**

```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: skynet-local
spec:
  cidrs:
  - 100.0.0.0/16    # Cluster's VTEP CIDR (from operator)
  mode: Managed
```

**VTEP IP Allocation:**
- Uses cluster's VTEP CIDR (e.g., 100.0.0.0/16)
- Allocates sequential IPs for each node
- Node 1: 100.0.0.1, Node 2: 100.0.0.2, etc.

**Note:** Remote cluster VTEPs are NOT created by agent. Remote endpoint info comes from broker sync.

---

### 7. pkg/operator/ (Operator Logic)

**Operator is responsible for resource allocation ONLY:**

#### pkg/operator/alloc/asn.go
- Allocates unique ASN per cluster from pool
- Default pool: 64512-64799
- Uses optimistic locking (read-modify-write with resourceVersion)

#### pkg/operator/alloc/vtep.go
- Allocates unique VTEP CIDR per cluster
- Default pool: 100.0.0.0/8 with /16 allocations
- Non-overlapping CIDRs guaranteed

#### pkg/operator/skynet/reconciler.go
- Watches Skynet CR (local cluster config)
- When Skynet CR is created:
  1. Allocate ASN if not set
  2. Allocate VTEP CIDR if not set
  3. Create/update Cluster CR on broker with allocations
  4. Deploy agent resources (RBAC, Deployment)

**Key: Operator writes allocations to Cluster CR spec, Agent reads them**

---

### 8. pkg/apis/skynet.io/v1/types.go

**CRD Definitions**

#### Cluster CR (Broker resource)
```go
type ClusterSpec struct {
    ASN      int32   // Allocated by operator
    VTEPCIDR string  // Allocated by operator
    // ... metadata
}

type ClusterStatus struct {
    Endpoints []Endpoint  // Reported by agent
    LastHeartbeatTime metav1.Time
}
```

#### Skynet CR (Local cluster config)
```go
type SkynetSpec struct {
    ClusterID       string
    BrokerURL       string
    BrokerNamespace string
    // ... broker auth
}
```

#### MultiClusterNetwork CR (Network registry on broker)
#### MultiClusterNetworkConnect CR (Local network connection request)

---

## Deployment Flow: make deploy

Let's trace through the complete deployment flow step by step.

### Step 1: `make clean`

**Script:** `test/cleanup.sh`

```bash
# Delete KIND clusters
kind delete cluster --name cluster1
kind delete cluster --name cluster2

# Remove generated files
rm -f test/kubeconfig-*.yaml
rm -f test/broker-token-*.txt
rm -f test/broker-server.txt

# Remove docker images
docker rmi skynet-agent:latest
```

**Result:** Clean slate for fresh deployment

---

### Step 2: `make clusters`

**Script:** `test/setup-clusters-v2.sh`

This is the most complex step. Let's break it down:

#### 2.1: Clone OVN-Kubernetes from GitHub

```bash
# Clone OVN-K to temporary directory
TMP_DIR=/tmp/ovn-kubernetes-skynet-XXXXX
git clone https://github.com/ovn-org/ovn-kubernetes.git $TMP_DIR
cd $TMP_DIR
```

**Why?** To get latest OVN-K with VTEP CRD support. Uses Shipyard pattern (clone on demand).

#### 2.2: Build OVN-K from Source

```bash
cd $TMP_DIR/contrib
./kind.sh --build
```

**Why build from source?**
- Latest VTEP CRD support
- Latest EVPN features
- Ensures compatibility

**What kind.sh does:**
- Builds OVN-K container images
- Patches configurations for KIND
- Prepares manifests

#### 2.3: Create Cluster1 (3 nodes)

```bash
export KUBECONFIG=$HOME/cluster1.conf
export KIND_CLUSTER_NAME=cluster1
export KIND_NUM_WORKER=2  # 1 control-plane + 2 workers = 3 nodes

# Create cluster with OVN-K
./kind.sh --install-cni-plugins
```

**Created resources:**
- KIND cluster "cluster1"
- OVN-K CNI installed
- OVN central + node components
- VTEP CRD installed

**Network:**
- Cluster CIDR: 10.244.0.0/16
- Service CIDR: 10.96.0.0/12
- Docker network: 172.18.0.0/16

#### 2.4: Install FRR-K8s on Cluster1

```bash
kubectl apply -f https://github.com/jcaamano/frr-k8s/archive/refs/heads/ovnk-bgp-v0.0.21.tar.gz
```

**FRR-K8s components installed:**
- FRRConfiguration CRD
- FRR daemon (DaemonSet on every node)
- FRR controller
- Webhook for validation

**What is FRR-K8s?**
- Kubernetes operator for FRRouting (BGP daemon)
- Manages FRR configuration via FRRConfiguration CR
- Runs FRR in containers on each node
- Used for BGP peering between clusters

#### 2.5: Create Cluster2 (same as cluster1)

```bash
export KUBECONFIG=$HOME/cluster2.conf
export KIND_CLUSTER_NAME=cluster2
# ... repeat cluster creation
```

**Result:** Two identical clusters with OVN-K + FRR-K8s

#### 2.6: Setup Broker Infrastructure

**Broker = Cluster1 (reusing cluster1 as broker)**

```bash
# Create broker namespace on cluster1
kubectl --context kind-cluster1 create namespace skynet-broker

# Install SkyNet CRDs on broker
kubectl --context kind-cluster1 apply -f deploy/crds/

# Create broker pool configuration
kubectl --context kind-cluster1 apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: skynet-broker-pools
  namespace: skynet-broker
data:
  asn-pool: "64512-64799"
  vtep-pool: "100.0.0.0/8"
  vtep-block-size: "16"
EOF
```

**What this creates:**
- Broker namespace (storage for Cluster CRs)
- CRDs: Cluster, MultiClusterNetwork, MultiClusterNetworkConnect, Skynet
- Pool configuration for operator allocations

#### 2.7: Create Agent RBAC on Both Clusters

**On each cluster:**

```yaml
# Namespace for agent
apiVersion: v1
kind: Namespace
metadata:
  name: skynet-operator

# ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skynet-agent
  namespace: skynet-operator

# ClusterRole (full permissions for agent)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: skynet-agent
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]

# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: skynet-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: skynet-agent
subjects:
- kind: ServiceAccount
  name: skynet-agent
  namespace: skynet-operator
```

#### 2.8: Create Broker Access Tokens

**For each cluster, on broker:**

```bash
# Create dedicated ServiceAccount on broker for cluster1
kubectl --context kind-cluster1 -n skynet-broker create sa skynet-agent-cluster1

# Grant permissions
kubectl --context kind-cluster1 -n skynet-broker create role skynet-agent-cluster1 \
  --verb=* --resource=clusters,multiclusternetworks

kubectl --context kind-cluster1 -n skynet-broker create rolebinding skynet-agent-cluster1 \
  --role=skynet-agent-cluster1 --serviceaccount=skynet-broker:skynet-agent-cluster1

# Create token for agent to authenticate to broker
kubectl --context kind-cluster1 -n skynet-broker create token skynet-agent-cluster1 \
  --duration=87600h > test/broker-token-cluster1.txt

# Get broker API server URL
kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' \
  > test/broker-server.txt
```

**Files created:**
- `output/kubeconfig-cluster1.yaml` - Cluster1 admin kubeconfig
- `output/kubeconfig-cluster2.yaml` - Cluster2 admin kubeconfig
- `test/broker-token-cluster1.txt` - JWT token for cluster1 agent
- `test/broker-token-cluster2.txt` - JWT token for cluster2 agent
- `test/broker-server.txt` - Broker API server URL

**Result of make clusters:**
- 2 KIND clusters with OVN-K + FRR-K8s
- Broker namespace with CRDs + pools
- RBAC configured for agents
- Authentication tokens created

---

### Step 3: `make build-agent`

**Script:** `test/build-and-load.sh`

#### 3.1: Build Docker Image

```bash
docker build -f package/Dockerfile.agent -t skynet-agent:latest .
```

**Dockerfile.agent:**
```dockerfile
FROM golang:1.25 as builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY pkg/ pkg/
RUN CGO_ENABLED=0 go build -o skynet-agent ./cmd/agent

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/skynet-agent .
ENTRYPOINT ["/skynet-agent"]
```

**Result:** Docker image `skynet-agent:latest` (minimal, ~20MB)

#### 3.2: Load Image to KIND Clusters

```bash
kind load docker-image skynet-agent:latest --name cluster1
kind load docker-image skynet-agent:latest --name cluster2
```

**Why load?** KIND clusters can't pull from DockerHub. Images must be loaded into cluster's containerd.

---

### Step 4: `make deploy-agents`

**Script:** `test/deploy-agents.sh`

This is where the magic happens - deploying agents that will start the reconciliation.

#### 4.1: Apply CRDs to Local Clusters

```bash
# Install SkyNet CRDs on cluster1
kubectl --kubeconfig output/kubeconfig-cluster1.yaml apply -f deploy/crds/

# Install SkyNet CRDs on cluster2
kubectl --kubeconfig output/kubeconfig-cluster2.yaml apply -f deploy/crds/
```

**Why?** Agents need to create/read Cluster, MCNC CRs locally.

#### 4.2: Create Broker Secret on Cluster1

```bash
# Read broker token
BROKER_TOKEN=$(cat test/broker-token-cluster1.txt)
BROKER_SERVER=$(cat test/broker-server.txt)

# For cluster1, broker is in-cluster (kubernetes.default.svc)
# Create secret with broker credentials
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n skynet-operator create secret generic skynet-agent-broker \
  --from-literal=token=$BROKER_TOKEN \
  --from-literal=server="https://kubernetes.default.svc:443" \
  --from-literal=namespace=skynet-broker
```

**Why different servers?**
- Cluster1: Broker is in-cluster → `kubernetes.default.svc:443`
- Cluster2: Broker is remote → `https://172.18.0.3:6443` (cluster1's API)

#### 4.3: Create Agent ConfigMap

```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n skynet-operator create configmap skynet-agent-config \
  --from-literal=cluster-id=cluster1
```

**Configuration passed to agent:**
- Cluster ID
- Broker credentials (from secret)

#### 4.4: Deploy Agent on Cluster1

```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml apply -f test/manifests/skynet-agent/
```

**Manifest: test/manifests/skynet-agent/deployment.yaml:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: skynet-agent
  namespace: skynet-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: skynet-agent
  template:
    metadata:
      labels:
        app: skynet-agent
    spec:
      serviceAccountName: skynet-agent
      containers:
      - name: agent
        image: skynet-agent:latest
        imagePullPolicy: Never  # Use locally loaded image
        env:
        - name: CLUSTER_ID
          valueFrom:
            configMapKeyRef:
              name: skynet-agent-config
              key: cluster-id
        - name: BROKER_URL
          valueFrom:
            secretKeyRef:
              name: skynet-agent-broker
              key: server
        - name: BROKER_TOKEN
          valueFrom:
            secretKeyRef:
              name: skynet-agent-broker
              key: token
        - name: BROKER_NAMESPACE
          valueFrom:
            secretKeyRef:
              name: skynet-agent-broker
              key: namespace
```

#### 4.5: Wait for Agent Rollout

```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n skynet-operator rollout status deployment/skynet-agent
```

**Waits until:** Agent pod is Running and ready

#### 4.6: Repeat for Cluster2

Same process for cluster2 with cluster2-specific tokens.

**Result of make deploy-agents:**
- Agents running on both clusters
- Agents have broker credentials
- Agents start reconciliation loops

---

### Step 5: Post-Deployment (What Agents Do Automatically)

Once agents are deployed, they immediately start reconciling:

#### Cluster1 Agent Reconciliation (iteration 1):

```
T+0s: Agent starts
T+1s: First reconciliation
  ├─ Read from broker: Cluster CR "cluster1" doesn't exist
  ├─ Create Cluster CR on broker:
  │  └─ spec: empty (no ASN/VTEP yet - operator will allocate)
  ├─ List local nodes: 3 nodes found
  ├─ Extract node IPs: 172.18.0.2, 172.18.0.3, 172.18.0.4
  ├─ Wait for operator to allocate ASN/VTEP...
```

#### Operator Reconciliation (on broker/cluster1):

```
T+2s: Operator sees new Cluster CR "cluster1" without ASN/VTEP
  ├─ Allocate ASN: 64512 (first available from pool)
  ├─ Allocate VTEP CIDR: 100.0.0.0/16 (first /16 from pool)
  ├─ Update Cluster CR:
  │  ├─ spec.asn = 64512
  │  └─ spec.vtepCIDR = "100.0.0.0/16"
```

#### Cluster1 Agent Reconciliation (iteration 2):

```
T+30s: Next reconciliation (broker syncer detected update)
  ├─ Read from broker: Cluster CR now has ASN=64512, VTEP=100.0.0.0/16
  ├─ Allocate VTEP IPs for nodes:
  │  ├─ Node 1: 100.0.0.1
  │  ├─ Node 2: 100.0.0.2
  │  └─ Node 3: 100.0.0.3
  ├─ Update Cluster CR status.endpoints:
  │  └─ [{nodeName: "cluster1-control-plane", bgpPeerIP: "172.18.0.3", vtepIP: "100.0.0.1"}...]
  ├─ Create local VTEP:
  │  └─ spec.cidrs: ["100.0.0.0/16"]
  ├─ Fetch remote clusters: cluster2 found!
  ├─ Read cluster2 endpoints: 3 nodes
  ├─ Create FRRConfiguration:
  │  └─ neighbors: [172.18.0.5, 172.18.0.6, 172.18.0.7] (cluster2 nodes)
```

#### Cluster2 Agent (parallel, same flow):

```
T+0s: Agent starts
T+1s: Create Cluster CR "cluster2"
T+2s: Operator allocates ASN=64513, VTEP=100.1.0.0/16
T+30s: Report endpoints, create FRRConfiguration with cluster1 neighbors
```

#### BGP Session Establishment:

```
T+35s: FRR daemons on both clusters see FRRConfiguration
  ├─ Cluster1 FRR: Initiate connections to 172.18.0.5/6/7
  ├─ Cluster2 FRR: Initiate connections to 172.18.0.2/3/4
  └─ Problem: FRR can't listen on external IPs (hardcoded -A 127.0.0.1)
```

#### Manual Workaround:

```
# Run workaround script
./test/workaround-frr-bgp.sh
  ├─ Patch ConfigMap: bgpd_options "-A 127.0.0.1" → "-A 0.0.0.0"
  ├─ Restart FRR pods
  └─ Wait for rollout

T+40s: BGP sessions establish
  ✓ 6 sessions up (3x3 full mesh, but 6 unique sessions)
```

---

## Complete Data Flow Visualization

```
┌─────────────────────────────────────────────────────────────────┐
│                         Broker Cluster                          │
│                      (cluster1 reused)                          │
│                                                                 │
│  ┌────────────────────────────────────────────────────────┐   │
│  │  Namespace: skynet-broker                              │   │
│  │                                                         │   │
│  │  ConfigMap: skynet-broker-pools                        │   │
│  │    asn-pool: "64512-64799"                            │   │
│  │    vtep-pool: "100.0.0.0/8"                           │   │
│  │                                                         │   │
│  │  Cluster CR: cluster1                                  │   │
│  │    spec:                                               │   │
│  │      asn: 64512         ← Operator allocated          │   │
│  │      vtepCIDR: 100.0.0.0/16  ← Operator allocated    │   │
│  │    status:                                             │   │
│  │      endpoints:         ← Agent reported               │   │
│  │      - nodeName: cluster1-control-plane               │   │
│  │        bgpPeerIP: 172.18.0.3                          │   │
│  │        vtepIP: 100.0.0.1                              │   │
│  │      - nodeName: cluster1-worker                      │   │
│  │        bgpPeerIP: 172.18.0.2                          │   │
│  │        vtepIP: 100.0.0.2                              │   │
│  │      - nodeName: cluster1-worker2                     │   │
│  │        bgpPeerIP: 172.18.0.4                          │   │
│  │        vtepIP: 100.0.0.3                              │   │
│  │                                                         │   │
│  │  Cluster CR: cluster2                                  │   │
│  │    spec:                                               │   │
│  │      asn: 64513                                        │   │
│  │      vtepCIDR: 100.1.0.0/16                           │   │
│  │    status:                                             │   │
│  │      endpoints: [3 nodes from cluster2]               │   │
│  └────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ Broker Sync (Submariner Admiral)
                              │
       ┌──────────────────────┴──────────────────────┐
       │                                             │
       │                                             │
┌──────▼───────────────────┐              ┌─────────▼──────────────┐
│   Cluster1 (Local)       │              │   Cluster2 (Local)     │
│                          │              │                        │
│  Namespace: skynet-operator            Namespace: skynet-operator│
│                          │              │                        │
│  Pod: skynet-agent       │              │  Pod: skynet-agent     │
│  ┌────────────────────┐ │              │  ┌──────────────────┐ │
│  │ Agent Container    │ │              │  │ Agent Container  │ │
│  │                    │ │              │  │                  │ │
│  │ 1. Bootstrap       │ │              │  │ 1. Bootstrap     │ │
│  │ 2. Register        │ │              │  │ 2. Register      │ │
│  │ 3. Discover nodes  │ │              │  │ 3. Discover nodes│ │
│  │ 4. Report endpoints│ │              │  │ 4. Report endpts │ │
│  │ 5. Create VTEP     │ │              │  │ 5. Create VTEP   │ │
│  │ 6. Create FRRConfig│ │              │  │ 6. Create FRRCfg │ │
│  └────────────────────┘ │              │  └──────────────────┘ │
│           │              │              │           │            │
│           ▼              │              │           ▼            │
│  VTEP: skynet-local      │              │  VTEP: skynet-local    │
│    cidrs: [100.0.0.0/16] │              │    cidrs: [100.1.0.0/16│
│                          │              │                        │
│  Namespace: frr-k8s-system             Namespace: frr-k8s-system│
│                          │              │                        │
│  FRRConfiguration:       │              │  FRRConfiguration:     │
│    skynet-bgp-config     │              │    skynet-bgp-config   │
│    spec:                 │              │    spec:               │
│      bgp:                │              │      bgp:              │
│        routers:          │              │        routers:        │
│        - asn: 64512      │              │        - asn: 64513    │
│          neighbors:      │              │          neighbors:    │
│          - 172.18.0.5 ───┼──────BGP─────┼──────▶ (node)         │
│          - 172.18.0.6 ───┼──────BGP─────┼──────▶ (node)         │
│          - 172.18.0.7 ───┼──────BGP─────┼──────▶ (node)         │
│           │              │              │           │            │
│           ▼              │              │           ▼            │
│  DaemonSet: frr-k8s-daemon             DaemonSet: frr-k8s-daemon│
│    (3 pods, one per node)│              │    (3 pods/node)       │
│    - Runs FRR/BGP daemon │              │    - Runs FRR/BGP      │
│    - Port 179 listening  │              │    - Port 179 listen   │
└──────────────────────────┘              └────────────────────────┘
         172.18.0.2/3/4                         172.18.0.5/6/7
              (nodes)                                (nodes)
```

---

## Key Takeaways

### Architecture Principles

1. **Separation of Concerns:**
   - **Operator:** Allocates resources (ASN, VTEP CIDR)
   - **Agent:** Does everything else (discovery, config, reconciliation)
   - **Broker:** Pure storage (no controllers)

2. **Operator-First Allocation:**
   - Agent never allocates ASN/VTEP CIDR
   - Agent reads allocations from Cluster CR spec
   - Prevents conflicts and race conditions

3. **Broker Sync Pattern:**
   - Submariner Admiral for reliable sync
   - Cluster CRs synced bidirectionally
   - Local changes propagate to broker
   - Remote changes propagate to local

4. **High-Level API Usage:**
   - Agent uses FRRConfiguration (not FRR internals)
   - Agent uses VTEP CRD (not OVN commands)
   - Future: RouteAdvertisement for CUDN stretching

### Deployment Steps Summary

```
make deploy =
  ├─ make clean (delete clusters)
  ├─ make clusters
  │  ├─ Clone OVN-K from GitHub
  │  ├─ Build OVN-K from source
  │  ├─ Create cluster1 (3 nodes, OVN-K, FRR-K8s)
  │  ├─ Create cluster2 (3 nodes, OVN-K, FRR-K8s)
  │  ├─ Setup broker namespace + CRDs + pools
  │  ├─ Create agent RBAC
  │  └─ Generate broker tokens
  ├─ make build-agent
  │  ├─ Build skynet-agent:latest image
  │  └─ Load to both clusters
  └─ make deploy-agents
     ├─ Apply CRDs to clusters
     ├─ Create broker secrets
     ├─ Deploy agent on cluster1
     └─ Deploy agent on cluster2

Post-deploy:
  ├─ Agents register clusters
  ├─ Operator allocates ASN/VTEP
  ├─ Agents report endpoints
  ├─ Agents create FRRConfiguration
  ├─ Apply FRR workaround
  └─ BGP sessions establish ✓
```

### Current Phase: BGP Peering (Complete ✓)

- [x] Cluster registration
- [x] ASN/VTEP allocation
- [x] Endpoint discovery
- [x] VTEP resource creation
- [x] FRRConfiguration generation
- [x] BGP full mesh establishment

### Next Phase: CUDN Stretching

- [ ] CUDN integrator (fetch existing CUDN, inject VNI/RT)
- [ ] VNI allocator integration
- [ ] RouteAdvertisement CR creation
- [ ] L2VPN EVPN configuration
- [ ] Cross-cluster pod connectivity

---

## Debugging Tips

### Check Agent Logs
```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n skynet-operator logs -l app=skynet-agent -f
```

### Check Broker State
```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n skynet-broker get clusters -o yaml
```

### Check BGP Status
```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n frr-k8s-system exec <frr-pod> -c frr -- vtysh -c 'show bgp summary'
```

### Check FRRConfiguration
```bash
kubectl --kubeconfig output/kubeconfig-cluster1.yaml -n frr-k8s-system get frrconfiguration skynet-bgp-config -o yaml
```

### Full Verification
```bash
make verify-bgp
```

---

This document provides a complete understanding of how SkyNet is structured and deployed. The key is understanding the separation between Operator (allocation) and Agent (everything else), and how broker sync enables auto-discovery.
