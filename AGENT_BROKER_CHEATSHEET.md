# Agent-Broker Communication Cheatsheet
## For "Sequence Diagram: MCN Interaction Flow" Slide

---

## Overview: Submariner Pattern

MCN uses the **same architecture as Submariner** - proven in production for multi-cluster networking:
- **Broker** = Pure storage (just a Kubernetes cluster with CRDs)
- **Agents** = Do all the work (orchestration, allocation, configuration)
- **Communication** = Agents talk to broker via Kubernetes API

Think of it like: "Broker is a shared database, Agents are smart clients"

---

## 1. Initial Setup: Deploy Broker

**What happens:**
```bash
# On Cluster1 (chosen as broker)
kubectl create namespace skynet-broker
kubectl apply -f crds/  # Install MCN CRDs (Cluster, MCN, MCNC)
```

**Result:**
- Broker cluster has MCN CRDs installed
- Broker namespace `skynet-broker` created
- **No controllers running** - just storage ready

**Key Point:** Broker is NOT a special component - it's just a regular Kubernetes cluster that stores CRDs!

---

## 2. Create ServiceAccount for Agent Access to Broker

**What happens:**
```bash
# On Broker (Cluster1)
kubectl create serviceaccount skynet-agent-cluster2 -n skynet-broker
```

**What is a ServiceAccount?**
- Like a "user account" for applications (not humans)
- Agents use this identity to authenticate to broker
- Each cluster gets its own ServiceAccount

**Why per-cluster ServiceAccounts?**
- Security: cluster2 agent can only access its own resources
- Isolation: cluster2 can't see cluster1's secrets
- Auditing: know which cluster made which changes

---

## 3. Create RBAC Permissions on Broker

**What happens:**
```yaml
# On Broker
apiVersion: rbac.authorization.k8s.io/v1
kind: Role  # ← Namespace-scoped (not ClusterRole)
metadata:
  name: skynet-agent-cluster2
  namespace: skynet-broker
rules:
- apiGroups: ["skynet.io"]
  resources: ["clusters", "multiclusternetworks"]  # MCN CRDs
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: skynet-agent-cluster2
  namespace: skynet-broker
roleRef:
  kind: Role
  name: skynet-agent-cluster2
subjects:
- kind: ServiceAccount
  name: skynet-agent-cluster2
  namespace: skynet-broker
```

**What is RBAC?**
- **R**ole-**B**ased **A**ccess **C**ontrol
- Defines "who can do what"
- Like permissions in a file system

**Breaking it down:**
- **Role**: List of allowed actions (verbs) on resources
  - Resources: `clusters`, `multiclusternetworks` (the MCN CRDs)
  - Verbs: `get, create, update` (read and write)
- **RoleBinding**: Connects ServiceAccount to Role
  - "Give skynet-agent-cluster2 the permissions in skynet-agent-cluster2 Role"

**Why namespace-scoped Role (not ClusterRole)?**
- Agent only needs access to `skynet-broker` namespace
- Can't touch other namespaces or cluster-level resources
- **Security best practice**: least privilege

---

## 4. Generate Token for Agent

**What happens:**
```bash
# On Broker
kubectl create token skynet-agent-cluster2 \
  -n skynet-broker \
  --duration=87600h  # 10 years (for demo)
```

**Output:** A long JWT token like:
```
eyJhbGciOiJSUzI1NiIsImtpZCI6IjBXV...  (base64-encoded)
```

**What is a Token?**
- Like a "password" for the ServiceAccount
- JWT = JSON Web Token (industry standard)
- Contains: who you are, expiration, signature

**How does it work?**
1. Token is **signed** by broker's Kubernetes API server
2. Token proves "I am skynet-agent-cluster2 ServiceAccount"
3. Broker API validates signature and checks RBAC permissions

**Production Note:**
- Demo uses 10-year tokens for simplicity
- Production should use short-lived tokens (hours/days) with rotation
- Or use ServiceAccount token projection (automatic refresh)

---

## 5. Get Broker API Server URL

**What happens:**
```bash
# On Broker
kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}'
```

**Output:**
```
https://172.18.0.2:6443  # Kind cluster IP
# or
https://broker.example.com:443  # Production
```

**What is this?**
- The HTTPS endpoint of broker's Kubernetes API server
- Agents will connect to this URL to talk to broker
- Like a database connection string

**Why do we need this?**
- Agents run on different clusters (cluster2)
- Need to know "where is the broker?" to connect remotely

---

## 6. Pass Credentials to Agent (via Secret)

**What happens:**
```bash
# On Cluster2 (where agent runs)
kubectl create secret generic skynet-agent-broker \
  -n skynet-operator \
  --from-literal=token="eyJhbGciOiJSUzI1NiIsImtpZCI..." \
  --from-literal=server="https://172.18.0.2:6443" \
  --from-literal=ca.crt="-----BEGIN CERTIFICATE-----..."
```

**What is a Secret?**
- Kubernetes object that stores sensitive data (passwords, tokens)
- Base64 encoded (not encrypted by default!)
- Mounted as environment variables or files in pods

**What's in the Secret?**
- `token`: JWT token for authentication
- `server`: Broker API server URL
- `ca.crt`: Broker's CA certificate (to verify TLS connection)

**Why use a Secret?**
- Keeps credentials out of code
- Can be updated without rebuilding images
- Kubernetes-native way to manage sensitive data

**Agent Deployment reads it:**
```yaml
env:
- name: SKYNET_BROKER_TOKEN
  valueFrom:
    secretKeyRef:
      name: skynet-agent-broker
      key: token
- name: SKYNET_BROKER_API_SERVER
  valueFrom:
    secretKeyRef:
      name: skynet-agent-broker
      key: server
```

---

## 7. Agent Authentication Flow

**What happens when agent starts:**

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. Agent pod starts on Cluster2                                 │
│    - Reads SKYNET_BROKER_TOKEN from Secret                      │
│    - Reads SKYNET_BROKER_API_SERVER from Secret                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Agent creates Kubernetes client for broker                   │
│    - URL: https://172.18.0.2:6443                               │
│    - Auth: BearerToken = eyJhbGciOiJSUzI1NiIsImtpZCI...        │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. Agent makes first API call to broker                         │
│    GET https://172.18.0.2:6443/apis/skynet.io/v1/               │
│         namespaces/skynet-broker/clusters                       │
│    Headers: Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI..│
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. Broker API server validates token                            │
│    - Verifies JWT signature (signed by broker)                  │
│    - Extracts identity: ServiceAccount = skynet-agent-cluster2  │
│    - Checks RBAC: Does this SA have "get clusters" permission?  │
│    - Answer: YES (from RoleBinding)                             │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. Broker returns cluster list                                  │
│    HTTP 200 OK                                                  │
│    Body: {"items": [...cluster objects...]}                     │
└─────────────────────────────────────────────────────────────────┘
```

**Key Point:** This is **standard Kubernetes authentication** - nothing special!
- Same mechanism as `kubectl` uses
- Same RBAC system as regular users
- Just using ServiceAccount instead of user certificate

---

## 8. Local Cluster RBAC (for Agent)

**Agent also needs permissions on its own cluster:**

```yaml
# On Cluster2 (local cluster)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skynet-agent
  namespace: skynet-operator
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole  # ← Cluster-scoped (needs to manage OVN-K resources)
metadata:
  name: skynet-agent
rules:
- apiGroups: [""]
  resources: ["nodes", "namespaces"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["k8s.ovn.org"]
  resources: ["vteps", "userdefinednetworks", "routeadvertisements"]
  verbs: ["*"]  # Full access to OVN-K CRDs
- apiGroups: ["k8s.ovn.org"]
  resources: ["clusteruserdefinednetworks"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["frrk8s.metallb.io"]
  resources: ["frrconfigurations"]
  verbs: ["*"]  # Full access to FRR configs
```

**Why ClusterRole here (not Role)?**
- Agent needs to manage cluster-wide resources (VTEPs, CUDNs, Nodes)
- These aren't namespaced - they're cluster-scoped
- Must use ClusterRole + ClusterRoleBinding

**What can agent do locally?**
- Read nodes (to discover BGP endpoints)
- Create VTEP CRs (for tunnel configuration)
- Patch CUDNs with EVPN config
- Create FRRConfigurations (for BGP)
- Create RouteAdvertisements (for route distribution)

---

## 9. Dual-Identity Pattern

**Agent has TWO identities:**

```
┌─────────────────────────────────────────────────────────────┐
│ Cluster2 (Local)                                            │
│                                                             │
│  ┌───────────────────────────────────────┐                 │
│  │ Agent Pod                             │                 │
│  │                                       │                 │
│  │ Identity 1: ServiceAccount            │                 │
│  │   "skynet-agent"                      │                 │
│  │   in namespace "skynet-operator"      │                 │
│  │                                       │                 │
│  │ Permissions:                          │                 │
│  │   - Manage OVN-K resources (VTEP,     │                 │
│  │     CUDN, RouteAdvertisement)         │                 │
│  │   - Manage FRR resources              │                 │
│  │   - Read nodes                        │                 │
│  └───────────────┬───────────────────────┘                 │
│                  │                                         │
│                  │ Uses in-cluster auth                    │
│                  │ (automatic via mounted SA token)        │
│                  │                                         │
└──────────────────┼─────────────────────────────────────────┘
                   │
                   │ Remote connection via token
                   │
┌──────────────────▼─────────────────────────────────────────┐
│ Broker Cluster (Remote)                                    │
│                                                             │
│  ┌───────────────────────────────────────┐                 │
│  │ Namespace: skynet-broker              │                 │
│  │                                       │                 │
│  │ Identity 2: ServiceAccount            │                 │
│  │   "skynet-agent-cluster2"             │                 │
│  │   in namespace "skynet-broker"        │                 │
│  │                                       │                 │
│  │ Permissions (namespace-scoped):       │                 │
│  │   - Manage Cluster CRs                │                 │
│  │   - Manage MultiClusterNetwork CRs    │                 │
│  │   - Allocate VNI/VTEP/ASN            │                 │
│  └───────────────────────────────────────┘                 │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Why two identities?**
- **Local identity**: Manages infrastructure (OVN-K, FRR)
- **Remote identity**: Coordinates across clusters (via broker)
- **Security**: Broker identity can't touch local infrastructure

---

## 10. Complete Flow: Agent Registers Cluster

**Step-by-step with auth details:**

```
Agent starts
    │
    ├─> Read local cluster
    │   ├─> Identity: skynet-agent (local SA)
    │   ├─> API: GET /api/v1/nodes
    │   └─> Result: Node list with IPs
    │
    ├─> Build Cluster CR with endpoints
    │   └─> Data: {name: "cluster2", asn: 64513, endpoints: [...]}
    │
    ├─> Connect to broker
    │   ├─> Identity: skynet-agent-cluster2 (broker SA)
    │   ├─> Auth: Bearer token from Secret
    │   ├─> URL: https://172.18.0.2:6443
    │   └─> TLS: Verify using CA cert from Secret
    │
    ├─> Check if Cluster CR exists on broker
    │   ├─> API: GET /apis/skynet.io/v1/namespaces/skynet-broker/clusters/cluster2
    │   ├─> Auth validated by broker:
    │   │   ├─> Token signature verified ✓
    │   │   ├─> Identity: skynet-agent-cluster2 ✓
    │   │   ├─> RBAC check: Role allows "get clusters" ✓
    │   │   └─> Permission granted!
    │   └─> Result: 404 Not Found (first time)
    │
    └─> Create Cluster CR on broker
        ├─> API: POST /apis/skynet.io/v1/namespaces/skynet-broker/clusters
        ├─> Body: Cluster CR with node endpoints
        ├─> Auth validated by broker:
        │   ├─> Token signature verified ✓
        │   ├─> Identity: skynet-agent-cluster2 ✓
        │   ├─> RBAC check: Role allows "create clusters" ✓
        │   └─> Permission granted!
        └─> Result: 201 Created
```

**Every API call follows this pattern:**
1. Agent includes token in `Authorization` header
2. Broker validates token signature
3. Broker extracts ServiceAccount identity
4. Broker checks RBAC (does this SA have permission?)
5. Broker allows or denies request

---

## 11. Security Considerations

### ✅ What's Secure:

1. **Token-based auth**: Industry standard (OAuth2/JWT)
2. **RBAC enforcement**: Kubernetes-native authorization
3. **Namespace isolation**: Agents only access their namespace
4. **TLS encryption**: All traffic encrypted in transit
5. **No shared credentials**: Each cluster has unique ServiceAccount

### ⚠️ Production Improvements:

1. **Short-lived tokens**: Use 1-hour tokens with automatic rotation
   - Demo uses 10-year for simplicity
   - Production: ServiceAccount token projection or cert-based auth

2. **CA certificate validation**: Always validate broker's TLS cert
   - Demo has fallback to insecure (for Kind)
   - Production: Must provide valid CA cert

3. **Network policies**: Restrict which pods can talk to broker
   - Firewall rules on broker API server
   - Only allow agent pods, not all pods

4. **Audit logging**: Track who accessed broker and when
   - Kubernetes audit logs show all API calls
   - Monitor for suspicious activity

5. **Secret encryption at rest**: Enable Kubernetes secret encryption
   - Demo: Secrets stored as base64 (not encrypted)
   - Production: Enable encryption provider

---

## 12. Comparison with Other Approaches

### MCN (Submariner Pattern):
```
Pros:
  ✓ Kubernetes-native auth (RBAC, ServiceAccounts)
  ✓ No special authentication service needed
  ✓ Works with standard kubectl/client-go
  ✓ Proven in production (Submariner)
  ✓ Easy to debug (standard K8s tools)

Cons:
  ✗ Requires token management
  ✗ Need network connectivity to broker API
```

### Alternative: mTLS (Mutual TLS):
```
Pros:
  ✓ Certificate-based (no tokens)
  ✓ Automatic rotation (cert-manager)

Cons:
  ✗ Requires PKI infrastructure
  ✗ More complex certificate management
  ✗ Harder to integrate with RBAC
```

### Alternative: OAuth2/OIDC:
```
Pros:
  ✓ Standard web auth
  ✓ Integration with identity providers

Cons:
  ✗ Requires external IdP
  ✗ More complex for machine-to-machine
  ✗ Kubernetes doesn't enforce OIDC RBAC natively
```

**Why we chose Submariner pattern:**
- Leverages existing Kubernetes primitives
- No additional components (IdP, PKI)
- Proven at scale (Submariner in production)
- Simple to understand and debug

---

## 13. Quick Reference: Key Concepts

| Concept | What It Is | Why We Need It |
|---------|-----------|----------------|
| **ServiceAccount** | Identity for pods | Agent needs to authenticate |
| **Token** | Password for SA | Proves identity to broker |
| **RBAC** | Permission system | Controls what agent can do |
| **Role** | List of permissions | Defines allowed actions |
| **RoleBinding** | Connects SA to Role | Grants permissions to agent |
| **Secret** | Stores credentials | Securely passes token to pod |
| **Bearer Token** | Auth header format | Industry standard HTTP auth |
| **JWT** | Token format | Signed, tamper-proof token |
| **CA Certificate** | TLS trust anchor | Verifies broker is authentic |

---

## 14. Troubleshooting Guide

### "Forbidden: cannot get resource 'clusters'"
```
Cause: RBAC permissions missing
Fix: Check Role includes the resource and verb
Debug: kubectl describe role skynet-agent-cluster2 -n skynet-broker
```

### "Unauthorized"
```
Cause: Token invalid or expired
Fix: Regenerate token and update Secret
Debug: kubectl create token skynet-agent-cluster2 -n skynet-broker
```

### "Connection refused"
```
Cause: Wrong broker API server URL
Fix: Verify broker server URL in Secret
Debug: kubectl --kubeconfig broker-kubeconfig config view --minify
```

### "x509: certificate signed by unknown authority"
```
Cause: CA certificate mismatch
Fix: Update CA cert in Secret to match broker
Debug: kubectl get secret skynet-agent-broker -o jsonpath='{.data.ca\.crt}' | base64 -d
```

---

## 15. Demo Script (for Presentation)

**Show authentication in action:**

```bash
# 1. Show ServiceAccount on broker
kubectl --context broker get sa skynet-agent-cluster2 -n skynet-broker

# 2. Show RBAC permissions
kubectl --context broker get role skynet-agent-cluster2 -n skynet-broker -o yaml

# 3. Show token (first 20 chars only!)
TOKEN=$(kubectl --context broker create token skynet-agent-cluster2 -n skynet-broker --duration=1h)
echo "Token: ${TOKEN:0:20}..."

# 4. Test authentication manually (prove it works)
kubectl --server=https://172.18.0.2:6443 \
        --token=$TOKEN \
        --insecure-skip-tls-verify \
        get clusters -n skynet-broker

# 5. Show Secret on agent cluster
kubectl --context cluster2 get secret skynet-agent-broker -n skynet-operator -o yaml

# 6. Show agent pod using the Secret
kubectl --context cluster2 get deploy skynet-agent -n skynet-operator -o yaml | grep -A 5 secretKeyRef
```

**Expected output:**
```
✓ ServiceAccount exists
✓ Role grants "get, list, create, update" on clusters
✓ Token works to list clusters
✓ Secret contains token and server URL
✓ Agent deployment mounts Secret as env vars
```

---

## 16. Presentation Talking Points

**Keep it simple for non-experts:**

🎯 **Main message:**
"Agent and broker communicate using standard Kubernetes authentication - the same way kubectl talks to any Kubernetes cluster."

📌 **Key points:**
1. **Broker is just a Kubernetes API** - nothing special
2. **Agents use ServiceAccount tokens** - like a password
3. **RBAC controls permissions** - what each agent can do
4. **Same pattern as Submariner** - proven in production
5. **No custom auth service needed** - leverages Kubernetes

🔐 **Security highlights:**
- Each cluster has isolated credentials
- Namespace-scoped permissions (least privilege)
- Token-based auth (industry standard)
- TLS encryption in transit

🚀 **Why this matters:**
- Simple to deploy (no extra infrastructure)
- Familiar to Kubernetes users
- Easy to debug (standard tools)
- Secure by default (RBAC enforced)

---

**Analogy for non-technical audience:**
"Think of it like a shared Google Drive folder:
- Broker = The folder that stores shared documents
- ServiceAccount = Your email address
- Token = Your password
- RBAC = Folder permissions (read-only vs. edit)
- Each cluster (team) has their own credentials but shares the folder"
