package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	oplmV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/medik8s/system-tests/tests/internal/labels"
	. "github.com/medik8s/system-tests/tests/internal/medik8sinittools"
	"github.com/medik8s/system-tests/tests/internal/medik8sparams"
	"github.com/medik8s/system-tests/tests/sbr-operator/internal/sbrparams"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe(
	"SBR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(sbrparams.Label), func() {
		var sbrDeployment *deployment.Builder

		BeforeAll(func() {
			By("Get SBR deployment object")

			var err error

			sbrDeployment, err = deployment.Pull(
				APIClient, sbrparams.OperatorDeploymentName, medik8sparams.OperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get SBR deployment")

			By("Verify SBR deployment is Ready")
			Expect(sbrDeployment.IsReady(medik8sparams.DefaultTimeout)).To(BeTrue(),
				"SBR deployment is not Ready")
		})

		It("Verify Storage-Based Remediation Operator pod is running",
			reportxml.ID("89232"), func() {
				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				_, err := pod.WaitForAllPodsInNamespaceRunning(
					APIClient,
					medik8sparams.OperatorNs,
					medik8sparams.DefaultTimeout,
					listOptions,
				)
				Expect(err).ToNot(HaveOccurred(), "Pod is not ready")

				By("Verifying pod count matches expected replicas")

				sbrPods, err := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
				Expect(err).ToNot(HaveOccurred(), "Failed to list SBR pods")

				var runningPods []*pod.Builder

				for _, p := range sbrPods {
					if p.Object.Status.Phase == corev1.PodRunning && p.Object.DeletionTimestamp == nil {
						runningPods = append(runningPods, p)
					}
				}

				infraConfig, err := infrastructure.Pull(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull infrastructure configuration")

				expectedCount := sbrparams.ExpectedReplicas
				if infraConfig.Object.Status.ControlPlaneTopology == configv1.SingleReplicaTopologyMode {
					expectedCount = int32(1)
				}

				Expect(int32(len(runningPods))).To(Equal(expectedCount),
					"Expected %d running SBR pod(s), found %d", expectedCount, len(runningPods))
			})

		It("Verify SBR CSV has required annotations",
			reportxml.ID("89233"), func() {
				By("Getting SBR ClusterServiceVersion")

				sbrCSVs, err := olm.ListClusterServiceVersionWithNamePattern(
					APIClient, "storage-based-remediation", medik8sparams.OperatorNs)
				Expect(err).ToNot(HaveOccurred(), "Failed to list SBR ClusterServiceVersions")
				Expect(len(sbrCSVs)).To(BeNumerically(">", 0),
					"At least one SBR ClusterServiceVersion should be found")

				By("Finding the active (Succeeded) CSV")

				var sbrCSV *olm.ClusterServiceVersionBuilder

				for _, csv := range sbrCSVs {
					phase, phaseErr := csv.GetPhase()
					if phaseErr == nil && phase == oplmV1alpha1.CSVPhaseSucceeded {
						sbrCSV = csv

						break
					}
				}

				Expect(sbrCSV).ToNot(BeNil(), "No SBR CSV in Succeeded phase found")

				By("Checking annotation values on SBR CSV")

				Expect(sbrCSV.Object.Annotations).ToNot(BeNil(), "CSV annotations should not be nil")

				for annotationKey, expectedValue := range sbrparams.RequiredAnnotations {
					annotationValue, exists := sbrCSV.Object.Annotations[annotationKey]
					Expect(exists).To(BeTrue(),
						"Required annotation %q should exist on SBR CSV", annotationKey)
					Expect(annotationValue).To(Equal(expectedValue),
						"Annotation %q should have value %q", annotationKey, expectedValue)
				}
			})

		It("Verify SBR controller manager has correct number of replicas",
			reportxml.ID("89234"), func() {
				By("Checking cluster topology")

				infraConfig, err := infrastructure.Pull(APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull infrastructure configuration")

				if infraConfig.Object.Status.ControlPlaneTopology == configv1.SingleReplicaTopologyMode {
					Skip("Skipping test on SNO (Single Node OpenShift) cluster")
				}

				By("Checking deployment replicas")
				Expect(sbrDeployment.Object.Spec.Replicas).ToNot(BeNil(),
					"Deployment replicas should not be nil")
				Expect(*sbrDeployment.Object.Spec.Replicas).To(Equal(sbrparams.ExpectedReplicas),
					"Expected %d replica(s), found %d",
					sbrparams.ExpectedReplicas, *sbrDeployment.Object.Spec.Replicas)

				By("Verifying ready replicas")
				Expect(sbrDeployment.Object.Status.ReadyReplicas).To(Equal(sbrparams.ExpectedReplicas),
					"Expected %d ready replica(s), found %d",
					sbrparams.ExpectedReplicas, sbrDeployment.Object.Status.ReadyReplicas)

				By("Verifying pods run on different nodes")

				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				sbrPods, err := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
				Expect(err).ToNot(HaveOccurred(), "Failed to list SBR pods")

				var runningPods []*pod.Builder

				for _, p := range sbrPods {
					if p.Object.Status.Phase == corev1.PodRunning && p.Object.DeletionTimestamp == nil {
						runningPods = append(runningPods, p)
					}
				}

				nodeNames := make(map[string]bool)

				for _, p := range runningPods {
					Expect(p.Object.Spec.NodeName).ToNot(BeEmpty(),
						"Pod %s has not been assigned to a node", p.Object.Name)
					nodeNames[p.Object.Spec.NodeName] = true
				}

				Expect(len(nodeNames)).To(Equal(int(sbrparams.ExpectedReplicas)),
					"SBR pods must run on different nodes for HA, but found pods on %d unique node(s)",
					len(nodeNames))
			})

		It("Verify SBR container runs as non-root user",
			reportxml.ID("89235"), func() {
				By("Getting SBR controller pod names")

				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				sbrPods, err := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
				Expect(err).ToNot(HaveOccurred(), "Failed to get SBR controller pods")

				var runningPods []*pod.Builder

				for _, p := range sbrPods {
					if p.Object.Status.Phase == corev1.PodRunning && p.Object.DeletionTimestamp == nil {
						runningPods = append(runningPods, p)
					}
				}

				Expect(len(runningPods)).To(BeNumerically(">", 0),
					"At least one running SBR controller pod should be found")

				var errorMessages []string

				for _, sbrPod := range runningPods {
					By(fmt.Sprintf("Verifying security context for pod %s", sbrPod.Object.Name))

					By("Checking pod-level runAsNonRoot security context")

					if sbrPod.Object.Spec.SecurityContext == nil {
						errorMessages = append(errorMessages,
							fmt.Sprintf("Pod %s has nil SecurityContext", sbrPod.Object.Name))
					} else if sbrPod.Object.Spec.SecurityContext.RunAsNonRoot == nil {
						errorMessages = append(errorMessages,
							fmt.Sprintf("Pod %s has nil runAsNonRoot", sbrPod.Object.Name))
					} else if !*sbrPod.Object.Spec.SecurityContext.RunAsNonRoot {
						errorMessages = append(errorMessages,
							fmt.Sprintf("Incorrect runAsNonRoot for pod %s. Expected true, found: %v",
								sbrPod.Object.Name,
								*sbrPod.Object.Spec.SecurityContext.RunAsNonRoot))
					}

					By("Checking manager container security context")

					managerFound := false

					for _, container := range sbrPod.Object.Spec.Containers {
						if container.Name != sbrparams.ManagerContainerName {
							continue
						}

						managerFound = true
						securityContext := container.SecurityContext

						if securityContext == nil {
							errorMessages = append(errorMessages,
								fmt.Sprintf("Container %s in pod %s has nil SecurityContext",
									container.Name, sbrPod.Object.Name))

							continue
						}

						if securityContext.RunAsUser != nil && *securityContext.RunAsUser == 0 {
							errorMessages = append(errorMessages,
								fmt.Sprintf("Container %s in pod %s runs as root (UID 0)",
									container.Name, sbrPod.Object.Name))
						}

						if securityContext.AllowPrivilegeEscalation == nil || *securityContext.AllowPrivilegeEscalation {
							errorMessages = append(errorMessages,
								fmt.Sprintf(
									"Container %s in pod %s: AllowPrivilegeEscalation must be explicitly false",
									container.Name, sbrPod.Object.Name))
						}

						if securityContext.Capabilities == nil {
							errorMessages = append(errorMessages,
								fmt.Sprintf(
									"Container %s in pod %s: Capabilities block is nil, must drop ALL",
									container.Name, sbrPod.Object.Name))
						} else {
							hasDropAll := false

							for _, cap := range securityContext.Capabilities.Drop {
								if cap == "ALL" {
									hasDropAll = true

									break
								}
							}

							if !hasDropAll {
								errorMessages = append(errorMessages,
									fmt.Sprintf("Container %s in pod %s does not drop ALL capabilities",
										container.Name, sbrPod.Object.Name))
							}
						}

						seccompOk := false
						if securityContext.SeccompProfile != nil &&
							securityContext.SeccompProfile.Type == corev1.SeccompProfileTypeRuntimeDefault {
							seccompOk = true
						} else if sbrPod.Object.Spec.SecurityContext != nil &&
							sbrPod.Object.Spec.SecurityContext.SeccompProfile != nil &&
							sbrPod.Object.Spec.SecurityContext.SeccompProfile.Type ==
								corev1.SeccompProfileTypeRuntimeDefault {
							seccompOk = true
						}

						if !seccompOk {
							errorMessages = append(errorMessages,
								fmt.Sprintf(
									"Container %s in pod %s missing RuntimeDefault seccomp profile",
									container.Name, sbrPod.Object.Name))
						}
					}

					if !managerFound {
						errorMessages = append(errorMessages,
							fmt.Sprintf("Pod %s has no container named %q",
								sbrPod.Object.Name, sbrparams.ManagerContainerName))
					}
				}

				if len(errorMessages) > 0 {
					errMsg := "Testing security context of SBR container failed due to:\n"
					for _, msg := range errorMessages {
						errMsg += fmt.Sprintf("- %s\n", msg)
					}

					Fail(errMsg)
				}
			})

		It("Verify SBR uses correct API and OLM naming",
			reportxml.ID("88822"),
			Label(
				labels.DisruptionNonDestructive,
				labels.TierSmoke,
				labels.PlatformAny,
				labels.ComponentOLM,
				labels.FrequencyPresubmit,
			), func() {
				By("Getting active SBR ClusterServiceVersion")

				sbrCSVs, err := olm.ListClusterServiceVersionWithNamePattern(
					APIClient, "storage-based-remediation", medik8sparams.OperatorNs)
				Expect(err).ToNot(HaveOccurred(), "Failed to list SBR CSVs")
				Expect(len(sbrCSVs)).To(BeNumerically(">", 0),
					"At least one SBR CSV should be found in namespace %s", medik8sparams.OperatorNs)

				var sbrCSV *olm.ClusterServiceVersionBuilder

				for _, csv := range sbrCSVs {
					phase, phaseErr := csv.GetPhase()
					if phaseErr == nil && phase == oplmV1alpha1.CSVPhaseSucceeded {
						sbrCSV = csv

						break
					}
				}

				Expect(sbrCSV).ToNot(BeNil(), "No SBR CSV in Succeeded phase found")

				By("Verifying CSV display name uses Storage-Based Remediation naming (not SBD)")
				Expect(sbrCSV.Object.Spec.DisplayName).To(ContainSubstring("Storage-Based Remediation"),
					"CSV display name should contain 'Storage-Based Remediation' (not 'SBD'), got: %q",
					sbrCSV.Object.Spec.DisplayName)
				Expect(sbrCSV.Object.Spec.DisplayName).ToNot(ContainSubstring("SBD"),
					"CSV display name should not use 'SBD' naming, got: %q",
					sbrCSV.Object.Spec.DisplayName)

				By(fmt.Sprintf("Verifying all owned CRDs use API group %s", sbrparams.CRDGroup))

				ownedCRDs := sbrCSV.Object.Spec.CustomResourceDefinitions.Owned
				Expect(ownedCRDs).ToNot(BeEmpty(), "CSV should declare at least one owned CRD")

				for _, expectedKind := range sbrparams.ExpectedCRDKinds {
					By(fmt.Sprintf("Checking owned CRD for kind %s", expectedKind))

					var matchedCRD *oplmV1alpha1.CRDDescription

					for i := range ownedCRDs {
						if ownedCRDs[i].Kind == expectedKind {
							matchedCRD = &ownedCRDs[i]

							break
						}
					}

					Expect(matchedCRD).ToNot(BeNil(),
						"CSV should own a CRD with kind %s", expectedKind)
					Expect(matchedCRD.Name).To(ContainSubstring(sbrparams.CRDGroup),
						"CRD %s name %q should include API group %s", expectedKind, matchedCRD.Name, sbrparams.CRDGroup)
					Expect(matchedCRD.Version).To(Equal(sbrparams.CRDVersion),
						"CRD %s should be at version %s", expectedKind, sbrparams.CRDVersion)
				}
			})
	})

var _ = Describe(
	"SBR Negative Tests",
	Ordered,
	ContinueOnFailure,
	Label(sbrparams.Label), func() {
		It("Verify StorageBasedRemediationConfig CR validation rejects invalid field values",
			reportxml.ID("88881"),
			Label(
				labels.DisruptionNonDestructive,
				labels.TierAcceptance,
				labels.PlatformAny,
				labels.ComponentWebhook,
				labels.FrequencyNightly,
			), func() {
				By("Layer 1: CRD OpenAPI schema — API server rejects out-of-range sbrTimeoutSeconds")

				type invalidSBRCCase struct {
					name  string
					field string
					value int64
				}

				for _, invalidCase := range []invalidSBRCCase{
					{"below-min-timeout", "sbrTimeoutSeconds", sbrparams.SBRCTimeoutSecondsMin - 1},
					{"above-max-timeout", "sbrTimeoutSeconds", sbrparams.SBRCTimeoutSecondsMax + 1},
					{"below-min-failures", "maxConsecutiveFailures", sbrparams.SBRCMaxConsecutiveFailuresMin - 1},
					{"above-max-failures", "maxConsecutiveFailures", sbrparams.SBRCMaxConsecutiveFailuresMax + 1},
				} {
					By(fmt.Sprintf("Attempting to create SBRC with %s=%d (expect rejection)",
						invalidCase.field, invalidCase.value))

					invalidSBRC := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": sbrparams.CRDGroup + "/" + sbrparams.CRDVersion,
							"kind":       "StorageBasedRemediationConfig",
							"metadata": map[string]interface{}{
								"name":      fmt.Sprintf("%s-%s", sbrparams.SBRCInvalidTestName, invalidCase.name),
								"namespace": medik8sparams.OperatorNs,
							},
							"spec": map[string]interface{}{
								invalidCase.field: invalidCase.value,
							},
						},
					}

					createErr := APIClient.Create(context.TODO(), invalidSBRC)
					Expect(createErr).To(HaveOccurred(),
						"API server should reject SBRC with %s=%d", invalidCase.field, invalidCase.value)
					Expect(k8serrors.IsInvalid(createErr)).To(BeTrue(),
						"Expected Invalid error for %s=%d, got: %v", invalidCase.field, invalidCase.value, createErr)
				}

				By("Layer 2: Controller validation — SBRC with non-existent StorageClass is admitted but DaemonSet is not deployed")

				sbrc := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": sbrparams.CRDGroup + "/" + sbrparams.CRDVersion,
						"kind":       "StorageBasedRemediationConfig",
						"metadata": map[string]interface{}{
							"name":      sbrparams.SBRCControllerTestName,
							"namespace": medik8sparams.OperatorNs,
						},
						"spec": map[string]interface{}{
							"sharedStorageClass": "nonexistent-storage-class",
						},
					},
				}

				err := APIClient.Create(context.TODO(), sbrc)
				Expect(err).ToNot(HaveOccurred(),
					"SBRC with invalid StorageClass reference should be admitted by API server")

				DeferCleanup(func() {
					By("Cleaning up controller-layer test SBRC")

					deleteErr := APIClient.Delete(context.TODO(), sbrc)
					if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
						GinkgoT().Logf("Warning: failed to delete test SBRC %s: %v",
							sbrparams.SBRCControllerTestName, deleteErr)
					}
				})

				By("Recording baseline DaemonSet count in operator namespace before reconciliation")

				baselineDSList, listErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
					context.TODO(), metav1.ListOptions{})
				Expect(listErr).ToNot(HaveOccurred(), "Failed to list DaemonSets in operator namespace")

				baselineCount := len(baselineDSList.Items)

				By("Verifying controller does not deploy a new DaemonSet for the invalid SBRC")

				Consistently(func() int {
					dsList, listErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
						context.TODO(), metav1.ListOptions{})
					if listErr != nil {
						return baselineCount
					}

					return len(dsList.Items)
				}, 30*time.Second, 5*time.Second).Should(Equal(baselineCount),
					"No new DaemonSet should appear for an SBRC with a non-existent StorageClass")
			})

		It("Verify SBRC controller handles invalid watchdog path and non-matching nodeSelector without scheduling agent pods",
			reportxml.ID("88741"),
			Label(
				labels.DisruptionNonDestructive,
				labels.TierAcceptance,
				labels.PlatformAny,
				labels.ComponentController,
				labels.FrequencyNightly,
			), func() {
				By("Recording baseline DaemonSet names before creating invalid SBRCs")

				baselineDSList, err := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
					context.TODO(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to list DaemonSets in operator namespace")

				baselineDSNames := make(map[string]bool, len(baselineDSList.Items))
				for _, ds := range baselineDSList.Items {
					baselineDSNames[ds.Name] = true
				}

				type invalidSBRCCase struct {
					name string
					spec map[string]interface{}
					desc string
				}

				for _, invalidCase := range []invalidSBRCCase{
					{
						name: sbrparams.SBRCWatchdogTestName,
						spec: map[string]interface{}{
							"watchdogPath": sbrparams.SBRCInvalidWatchdogPath,
						},
						desc: "invalid watchdog device path",
					},
					{
						name: sbrparams.SBRCNoMatchSelectorTestName,
						spec: map[string]interface{}{
							"nodeSelector": map[string]interface{}{
								sbrparams.SBRCNoMatchSelectorKey: sbrparams.SBRCNoMatchSelectorValue,
							},
						},
						desc: "nodeSelector matching no cluster nodes",
					},
				} {
					By(fmt.Sprintf("Creating SBRC with %s", invalidCase.desc))

					sbrc := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": sbrparams.CRDGroup + "/" + sbrparams.CRDVersion,
							"kind":       "StorageBasedRemediationConfig",
							"metadata": map[string]interface{}{
								"name":      invalidCase.name,
								"namespace": medik8sparams.OperatorNs,
							},
							"spec": invalidCase.spec,
						},
					}

					createErr := APIClient.Create(context.TODO(), sbrc)
					Expect(createErr).ToNot(HaveOccurred(),
						"SBRC with %s should be admitted by the API server", invalidCase.desc)

					sbrcRef := sbrc.DeepCopy()

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up test SBRC %s", sbrcRef.GetName()))

						deleteErr := APIClient.Delete(context.TODO(), sbrcRef)
						if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
							GinkgoT().Logf("Warning: failed to delete test SBRC %s: %v",
								sbrcRef.GetName(), deleteErr)
						}
					})

					By(fmt.Sprintf("Verifying controller does not schedule agent pods for SBRC with %s", invalidCase.desc))

					Consistently(func() error {
						dsList, listErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
							context.TODO(), metav1.ListOptions{})
						if listErr != nil {
							return listErr
						}

						for _, daemonSet := range dsList.Items {
							if baselineDSNames[daemonSet.Name] {
								continue
							}

							if daemonSet.Status.DesiredNumberScheduled > 0 {
								return fmt.Errorf("new DaemonSet %q has %d agent pod(s) scheduled; expected 0 for SBRC with %s",
									daemonSet.Name,
									daemonSet.Status.DesiredNumberScheduled,
									invalidCase.desc)
							}
						}

						return nil
					}, 30*time.Second, 5*time.Second).Should(Succeed(),
						"Controller must not schedule agent pods for SBRC with %s", invalidCase.desc)

					By(fmt.Sprintf("Verifying SBRC %s still exists after controller reconciliation", invalidCase.name))

					getErr := APIClient.Get(context.TODO(),
						types.NamespacedName{Name: invalidCase.name, Namespace: medik8sparams.OperatorNs},
						sbrcRef)
					Expect(getErr).ToNot(HaveOccurred(),
						"SBRC %q must still exist after controller reconciliation with %s",
						invalidCase.name, invalidCase.desc)
				}
			})
	})
