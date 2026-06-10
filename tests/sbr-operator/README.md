# SBR Operator Post-Deployment Tests

Automated tests validating the Storage-Based Remediation (SBR) operator
deployment, security posture, and high-availability configuration.

## Prerequisites

- OpenShift cluster with SBR operator installed via OLM
- `KUBECONFIG` set with cluster-admin access
- SBR installed in `openshift-workload-availability` namespace

## Running

```bash
ginkgo --label-filter="sbr" ./tests/sbr-operator/...
```

## Tests

### 1. Verify SBR Operator Pod is Running (Polarion 89232)

Validates that SBR controller-manager pods are in Running state and the
pod count matches the cluster topology (2 on multi-node, 1 on SNO).

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology (MNO or SNO)
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="pod is running" ./tests/sbr-operator/...`
- **Pass criteria**: All pods Running, count matches expected replicas

### 2. Verify SBR CSV Has Required Annotations (Polarion 89233)

Validates that the active SBR ClusterServiceVersion (in Succeeded phase)
has all required OLM feature annotations: disconnected support, FIPS
compliance flag, suggested namespace, and feature flags.

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="required annotations" ./tests/sbr-operator/...`
- **Pass criteria**: Required annotations present with expected values

### 3. Verify SBR Controller Replicas and Node Distribution (Polarion 89234)

Validates that 2 replicas are running and scheduled on different nodes
for high availability. Skipped on SNO clusters where only 1 replica is
expected.

- **Operators**: SBR v0.3.0
- **Cluster**: Multi-node only (skips on SNO)
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="correct number of replicas" ./tests/sbr-operator/...`
- **Pass criteria**: 2 ready replicas on 2 different nodes

### 4. Verify SBR Container Security Context (Polarion 89235)

Validates the manager container follows the restricted security posture:
runAsNonRoot at pod level, allowPrivilegeEscalation=false,
capabilities.drop=ALL, and seccompProfile=RuntimeDefault (at container
or pod level). Only checks the `manager` container, not sidecars.

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="non-root user" ./tests/sbr-operator/...`
- **Pass criteria**: All security context fields match restricted profile

### 5. Verify SBR Uses Correct API and OLM Naming (Polarion 88822)

Validates that the active SBR CSV display name uses "Storage-Based Remediation"
(not the legacy "SBD" branding) and that all owned CRDs are registered under the
correct API group `storage-based-remediation.medik8s.io`.

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="correct API and OLM naming" ./tests/sbr-operator/...`
- **Pass criteria**: CSV display name contains "Storage-Based Remediation", does not contain "SBD", all CRD API groups match expected value

### 6. Verify StorageBasedRemediationConfig CRD Schema Rejects Invalid Values (Polarion 88881)

Validates two layers of SBRC validation:

**Layer 1 (CRD OpenAPI schema)**: The API server rejects SBRC resources with
out-of-range field values for `sbrTimeoutSeconds` and `maxConsecutiveFailures`.

**Layer 2 (Controller validation)**: An SBRC referencing a non-existent
StorageClass is admitted by the API server but the controller does not schedule
a DaemonSet for it.

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="StorageBasedRemediationConfig" ./tests/sbr-operator/...`
- **Pass criteria**: Out-of-range SBRC fields rejected; invalid-StorageClass SBRC admitted but no DaemonSet created

### 7. Verify SBRC Controller Handles Invalid Inputs Without Scheduling Agent Pods (Polarion 88741)

Validates that the SBR controller does not schedule agent DaemonSets when
`StorageBasedRemediationConfig` resources specify inputs the controller cannot
act on:

- **Invalid watchdog path**: SBRC with a non-existent watchdog device path
  (`/dev/sbr-test-nonexistent-watchdog`) is admitted by the API server but the
  controller schedules no DaemonSet.
- **Non-matching nodeSelector**: SBRC with a nodeSelector that matches no cluster
  nodes is admitted and may produce a DaemonSet, but `DesiredNumberScheduled`
  must remain 0 for the duration of the observation window.

- **Operators**: SBR v0.3.0
- **Cluster**: Any topology
- **Storage**: None
- **Environment**: Connected or disconnected
- **Standalone**: `ginkgo --label-filter="sbr" --focus="invalid watchdog path and non-matching nodeSelector" ./tests/sbr-operator/...`
- **Pass criteria**: No agent pods scheduled for either invalid SBRC input; SBRCs remain present after controller reconciliation
