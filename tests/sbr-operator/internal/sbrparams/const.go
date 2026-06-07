package sbrparams

import "time"

const (
	// DefaultPollInterval is the polling interval used with Eventually calls.
	DefaultPollInterval = 5 * time.Second

	// SBRCConsistentlyDuration is how long negative tests observe the controller for unexpected DaemonSets.
	SBRCConsistentlyDuration = 30 * time.Second

	// SBRCConsistentlyPollInterval is the polling interval used with SBRCConsistentlyDuration.
	SBRCConsistentlyPollInterval = 5 * time.Second

	// Label represents SBR operator label that can be used for test cases selection.
	Label = "sbr"

	// ExpectedReplicas defines the expected number of replicas for SBR controller manager.
	ExpectedReplicas = int32(2)

	// ManagerContainerName is the name of the main controller container in the SBR pod.
	ManagerContainerName = "manager"

	// CRDGroup is the Kubernetes API group for all SBR custom resources.
	CRDGroup = "storage-based-remediation.medik8s.io"

	// CRDVersion is the API version for all SBR custom resources.
	CRDVersion = "v1alpha1"

	// SBRCTimeoutSecondsMin is the minimum allowed value for sbrTimeoutSeconds (CRD schema enforced).
	SBRCTimeoutSecondsMin = 10

	// SBRCTimeoutSecondsMax is the maximum allowed value for sbrTimeoutSeconds (CRD schema enforced).
	SBRCTimeoutSecondsMax = 300

	// SBRCMaxConsecutiveFailuresMin is the minimum allowed value for maxConsecutiveFailures (CRD schema enforced).
	SBRCMaxConsecutiveFailuresMin = 2

	// SBRCMaxConsecutiveFailuresMax is the maximum allowed value for maxConsecutiveFailures (CRD schema enforced).
	SBRCMaxConsecutiveFailuresMax = 32

	// SBRCInvalidTestName is the name used for short-lived invalid SBRC CRs in negative tests.
	SBRCInvalidTestName = "test-invalid-sbrc"

	// SBRCControllerTestName is the name used for SBRC CRs testing controller-layer validation.
	SBRCControllerTestName = "test-controller-invalid-sbrc"
)
