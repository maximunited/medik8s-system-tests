package labels

const (
	// Operator axis — which operator(s) are tested.
	OperatorSBR     = "operator:sbr"
	OperatorSNR     = "operator:snr"
	OperatorNHC     = "operator:nhc"
	OperatorFAR     = "operator:far"
	OperatorMDR     = "operator:mdr"
	OperatorNMO     = "operator:nmo"
	OperatorInterop = "operator:interop"

	// Tier axis — test importance / depth.
	TierSmoke      = "tier:smoke"
	TierAcceptance = "tier:acceptance"
	TierInterop    = "tier:interop"
	TierUpgrade    = "tier:upgrade"
	TierResiliency = "tier:resiliency"

	// Frequency axis — when the test should run.
	FrequencyPresubmit = "frequency:presubmit"
	FrequencyNightly   = "frequency:nightly"
	FrequencyWeekly    = "frequency:weekly"
	FrequencyRelease   = "frequency:release"

	// Disruption axis — whether the test reboots/drains nodes.
	DisruptionDestructive    = "disruption:destructive"
	DisruptionNonDestructive = "disruption:nondestructive"

	// Component axis — what part of the system is under test.
	ComponentOLM         = "component:olm"
	ComponentController  = "component:controller"
	ComponentDaemonSet   = "component:daemonset"
	ComponentWebhook     = "component:webhook"
	ComponentRemediation = "component:remediation"
	ComponentMetrics     = "component:metrics"

	// Platform axis — infrastructure requirements.
	PlatformAWS       = "platform:aws"
	PlatformBareMetal = "platform:baremetal"
	PlatformAny       = "platform:any"
)
