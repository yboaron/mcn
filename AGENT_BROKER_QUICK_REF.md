# Agent-Broker Quick Reference
## For Presentation - Simple Visual Guide

---

## The Big Picture (30 seconds)

```
┌──────────────┐                    ┌──────────────┐
│  Cluster 2   │                    │  Cluster 1   │
│              │                    │  (Broker)    │
│  ┌────────┐  │                    │              │
│  │ Agent  │──┼────── Token ──────▶│  Kubernetes  │
│  │        │  │    over HTTPS      │  API Server  │
│  └────────┘  │                    │              │
│              │                    │  ┌────────┐  │
│              │                    │  │  CRDs  │  │
│              │                    │  │ Storage│  │
└──────────────┘                    └──┴────────┴──┘

"Agent is a client, Broker is a server - just Kubernetes API calls"
```

---

## Authentication Flow (3 steps)

```
1. SETUP (done once)
   ├─ Create ServiceAccount on broker
   ├─ Create RBAC permissions
   └─ Generate token

2. DEPLOY (agent startup)
   ├─ Pass token to agent via Secret
   └─ Agent reads token from environment

3. RUNTIME (every API call)
   ├─ Agent: "Here's my token"
   ├─ Broker: "Token valid? ✓ RBAC allows? ✓"
   └─ Broker: "OK, here's your data"
```

---

## Key Components

### ServiceAccount (Identity)
```
Like: An email address
Purpose: Identifies the agent
Example: skynet-agent-cluster2
```

### Token (Password)
```
Like: A password
Purpose: Proves you are the ServiceAccount
Example: eyJhbGciOiJSUzI1NiIsImtpZCI6IjBXV... (JWT)
Lifetime: 10 years (demo) / 1 hour (production)
```

### RBAC (Permissions)
```
Like: Folder permissions (read-only vs edit)
Purpose: Controls what agent can do
Example: "Can create/update Cluster CRs in skynet-broker namespace"
```

### Secret (Secure Storage)
```
Like: Password manager
Purpose: Securely pass token to agent pod
Contains: token, broker URL, CA certificate
```

---

## RBAC Explained (Simple)

```yaml
# Role: What actions are allowed?
Role:
  - Resource: clusters, multiclusternetworks
  - Verbs: get, list, create, update
  - Translation: "Can read and write Cluster/MCN CRs"

# RoleBinding: Who gets these permissions?
RoleBinding:
  - ServiceAccount: skynet-agent-cluster2
  - Role: skynet-agent-cluster2
  - Translation: "Give the agent these permissions"
```

**Result:** Agent can manage CRs in `skynet-broker` namespace, nothing else

---

## Dual Identity (Important!)

```
Agent has 2 ServiceAccounts:

┌─────────────────────────────────────────────────┐
│ LOCAL (Cluster 2)                               │
│                                                 │
│ ServiceAccount: skynet-agent                    │
│ Namespace: skynet-operator                      │
│ Permissions: Manage OVN-K, FRR resources        │
│              (VTEP, CUDN, FRRConfiguration)     │
│ Scope: ClusterRole (cluster-wide access)        │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ REMOTE (Broker)                                 │
│                                                 │
│ ServiceAccount: skynet-agent-cluster2           │
│ Namespace: skynet-broker                        │
│ Permissions: Manage MCN CRDs                    │
│              (Cluster, MultiClusterNetwork)     │
│ Scope: Role (namespace-only access)             │
└─────────────────────────────────────────────────┘

Why? Security isolation - broker identity can't touch local infrastructure
```

---

## Security Highlights

### ✅ What's Good:
- **Token-based auth** - industry standard (OAuth2/JWT)
- **RBAC enforced** - Kubernetes-native authorization  
- **Namespace isolation** - agent only sees skynet-broker namespace
- **TLS encrypted** - all traffic over HTTPS
- **Unique credentials** - each cluster has different ServiceAccount/token

### 🔐 Production Best Practices:
- Use **short-lived tokens** (1 hour) with auto-rotation
- Validate **CA certificates** (don't skip TLS verification)
- Enable **secret encryption at rest**
- Apply **network policies** (firewall broker API)
- Monitor **audit logs** (track API access)

---

## Example: Agent Registers Cluster

```
Step 1: Agent reads local nodes
  └─> Uses: Local ServiceAccount (skynet-agent)
  └─> API: GET /api/v1/nodes
  └─> Result: [node1: 172.18.0.2, node2: 172.18.0.3]

Step 2: Agent builds Cluster CR
  └─> Data: {name: cluster2, asn: 64513, endpoints: [...]}

Step 3: Agent creates CR on broker
  └─> Uses: Remote ServiceAccount (skynet-agent-cluster2)
  └─> Auth: Bearer token from Secret
  └─> API: POST /apis/skynet.io/v1/namespaces/skynet-broker/clusters
  └─> Headers: Authorization: Bearer eyJhbGciOiJSUzI1NiIsImtpZCI...

Step 4: Broker validates request
  ├─> Verify token signature ✓
  ├─> Extract identity: skynet-agent-cluster2 ✓
  ├─> Check RBAC: Does SA have "create clusters"? ✓
  └─> Allow request ✓

Step 5: Broker stores Cluster CR
  └─> Result: 201 Created
```

**Key Point:** Every API call is authenticated and authorized by Kubernetes!

---

## Comparison with Alternatives

| Approach | MCN (Submariner) | mTLS | OAuth2/OIDC |
|----------|------------------|------|-------------|
| **Auth Method** | ServiceAccount Token | Client Certificate | OAuth tokens |
| **Infrastructure** | None (built-in K8s) | PKI/cert-manager | External IdP |
| **RBAC Integration** | Native | Manual mapping | Complex |
| **Rotation** | Manual (or projection) | Automatic | Automatic |
| **Complexity** | Low | Medium | High |
| **K8s Native** | ✅ Yes | Partial | No |
| **Production Use** | Submariner | Istio | Web apps |

**Why we chose Submariner pattern:**
- ✅ Leverages existing Kubernetes authentication
- ✅ No additional components needed
- ✅ Proven at scale (Submariner)
- ✅ Simple to understand and debug

---

## Common Questions & Answers

**Q: Is the token secure?**
A: Yes - it's a signed JWT. Broker verifies signature on every request. Can't be forged.

**Q: What if token is stolen?**
A: Token only grants access to skynet-broker namespace (RBAC limited). Rotate token immediately.

**Q: Why not use certificates?**
A: Tokens are simpler for this use case. Certificates require PKI infrastructure. Both are secure.

**Q: How does agent know broker URL?**
A: Passed via Secret as environment variable. Set during deployment.

**Q: Can cluster1 agent access cluster2 resources?**
A: No - each cluster has separate ServiceAccount. cluster1 agent has "skynet-agent-cluster1" identity.

**Q: What if broker is down?**
A: Agent retries with exponential backoff. Local infrastructure keeps working. Sync resumes when broker returns.

**Q: Is this the same as Submariner?**
A: Yes! Exact same authentication pattern. Proven in production multi-cluster deployments.

---

## Demo Commands (for Live Presentation)

```bash
# 1. Show ServiceAccount on broker
kubectl get sa -n skynet-broker
# Output: skynet-agent-cluster1, skynet-agent-cluster2

# 2. Show RBAC (what can agent do?)
kubectl get role skynet-agent-cluster2 -n skynet-broker -o yaml
# Output: Can manage clusters, multiclusternetworks

# 3. Generate token (shows it's just kubectl!)
kubectl create token skynet-agent-cluster2 -n skynet-broker --duration=5m
# Output: eyJhbGciOiJSUzI1NiIsImtpZCI6...

# 4. Show Secret with token
kubectl get secret skynet-agent-broker -n skynet-operator -o yaml
# Output: token, server URL, CA cert

# 5. Show agent pod using Secret
kubectl describe deploy skynet-agent -n skynet-operator | grep -A 3 "Environment"
# Output: SKYNET_BROKER_TOKEN from secret

# 6. Show agent successfully registered
kubectl get clusters -n skynet-broker
# Output: cluster1, cluster2 ✓
```

---

## Visual: Authentication Sequence

```
Time: T0 - Setup
┌────────┐
│ Admin  │
└───┬────┘
    │
    ├─> Create SA "skynet-agent-cluster2" on broker
    ├─> Create Role with "manage clusters" permission
    ├─> Create RoleBinding (SA → Role)
    └─> Generate token (kubectl create token)

Time: T1 - Deployment
┌────────┐
│ Admin  │
└───┬────┘
    │
    └─> Create Secret with token on cluster2
        └─> Deploy agent with Secret mounted

Time: T2 - Runtime (every 30 seconds)
┌────────┐                              ┌────────┐
│ Agent  │                              │ Broker │
└───┬────┘                              └───┬────┘
    │                                       │
    ├─ GET /clusters (with token) ────────▶│
    │                                       ├─ Validate token ✓
    │                                       ├─ Check RBAC ✓
    │◀──────── 200 OK [{cluster1}, ...] ───┤
    │                                       │
    ├─ PATCH /clusters/cluster2 (update) ─▶│
    │                                       ├─ Validate token ✓
    │                                       ├─ Check RBAC ✓
    │◀──────── 200 OK (updated) ───────────┤
    │                                       │
```

---

## Analogy for Non-Technical Audience

**"Think of it like a company shared drive"**

- **Broker** = Shared Google Drive folder
- **ServiceAccount** = Your company email (alice@company.com)
- **Token** = Your password
- **RBAC** = Folder permissions (read-only vs can edit)
- **Secret** = Password saved in your browser
- **Agent** = You, accessing the drive from your laptop

**What happens:**
1. IT creates your email account (ServiceAccount)
2. IT gives you folder access (RBAC Role)
3. You save your password (Secret)
4. Every time you open a file, Drive checks your password and permissions
5. If valid → you see the file. If not → access denied.

**MCN is exactly the same** - just using Kubernetes API instead of Google Drive!

---

## Key Takeaway for Presentation

🎯 **Main Message:**
"MCN agents talk to broker using **standard Kubernetes authentication** - the same mechanism kubectl uses. No custom auth service, no special infrastructure needed."

📌 **Supporting Points:**
- Leverages built-in Kubernetes RBAC
- Same pattern as Submariner (proven at scale)
- Simple to deploy and debug
- Secure by default

🔐 **Security:**
- Token-based authentication (industry standard)
- Namespace-scoped permissions (least privilege)
- Each cluster has isolated credentials

**Bottom Line:** If you understand how kubectl works, you understand how MCN authentication works!
