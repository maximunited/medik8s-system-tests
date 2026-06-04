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

func findActiveCSV(csvs []*olm.ClusterServiceVersionBuilder) *olm.ClusterServiceVersionBuilder {
	for _, csv := range csvs {
		phase, err := csv.GetPhase()
		if err == nil && phase == oplmV1alpha1.CSVPhaseSucceeded {
			return csv
		}
	}

	return nil
}

func filterRunningPods(pods []*pod.Builder) []*pod.Builder {
	var running []*pod.Builder

	for _, p := range pods {
		if p.Object.Status.Phase == corev1.PodRunning && p.Object.DeletionTimestamp == nil {
			running = append(running, p)
		}
	}

	return running
}

func fetchActiveCSV() *olm.ClusterServiceVersionBuilder {
	sbrCSVs, err := olm.ListClusterServiceVersionWithNamePattern(
		APIClient, "storage-based-remediation", medik8sparams.OperatorNs)
	Expect(err).ToNot(HaveOccurred(), "Failed to list SBR ClusterServiceVersions")
	Expect(len(sbrCSVs)).To(BeNumerically(">", 0),
		"At least one SBR ClusterServiceVersion should be found in namespace %s", medik8sparams.OperatorNs)

	sbrCSV := findActiveCSV(sbrCSVs)
	Expect(sbrCSV).ToNot(BeNil(), "No SBR CSV in Succeeded phase found")

	return sbrCSV
}

func buildSBRC(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": sbrparams.CRDGroup + "/" + sbrparams.CRDVersion,
			"kind":       "StorageBasedRemediationConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": medik8sparams.OperatorNs,
			},
			"spec": spec,
		},
	}
}

var _ = Describe(
	"SBR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(sbrparams.Label), func() {
		var controlPlaneTopology configv1.TopologyMode

		BeforeAll(func() {
			By("Get SBR deployment object and verify it is Ready")

			sbrDeployment, err := deployment.Pull(
				APIClient, sbrparams.OperatorDeploymentName, medik8sparams.OperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get SBR deployment")
			Expect(sbrDeployment.IsReady(medik8sparams.DefaultTimeout)).To(BeTrue(),
				"SBR deployment is not Ready")

			By("Pull cluster topology for use in topology-aware tests")

			infraConfig, infraErr := infrastructure.Pull(APIClient)
			Expect(infraErr).ToNot(HaveOccurred(), "Failed to pull infrastructure configuration")

			controlPlaneTopology = infraConfig.Object.Status.ControlPlaneTopology
		})

		It("Verify Storage-Based Remediation Operator pod is running",
			reportxml.ID("89232"), func() {
				expectedCount := sbrparams.ExpectedReplicas
				if controlPlaneTopology == configv1.SingleReplicaTopologyMode {
					expectedCount = int32(1)
				}

				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				By("Verifying pod count matches expected replicas")

				Eventually(func() error {
					sbrPods, listErr := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
					if listErr != nil {
						return listErr
					}

					runningCount := int32(len(filterRunningPods(sbrPods)))

					if runningCount != expectedCount {
						return fmt.Errorf("expected %d running SBR pod(s), found %d",
							expectedCount, runningCount)
					}

					return nil
				}, medik8sparams.DefaultTimeout, 5*time.Second).Should(Succeed(),
					"SBR pods did not reach expected running count of %d", expectedCount)
			})

		It("Verify SBR CSV has required annotations",
			reportxml.ID("89233"), func() {
				By("Getting SBR ClusterServiceVersion")

				By("Finding the active (Succeeded) CSV")

				sbrCSV := fetchActiveCSV()

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

				if controlPlaneTopology == configv1.SingleReplicaTopologyMode {
					Skip("Skipping test on SNO (Single Node OpenShift) cluster")
				}

				By("Verifying replica count, ready replicas, and pod HA distribution")

				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				Eventually(func() error {
					liveDeploy, pullErr := deployment.Pull(
						APIClient, sbrparams.OperatorDeploymentName, medik8sparams.OperatorNs)
					if pullErr != nil {
						return pullErr
					}

					if liveDeploy.Object.Spec.Replicas == nil ||
						*liveDeploy.Object.Spec.Replicas != sbrparams.ExpectedReplicas {
						desired := int32(0)
						if liveDeploy.Object.Spec.Replicas != nil {
							desired = *liveDeploy.Object.Spec.Replicas
						}

						return fmt.Errorf("expected %d desired replica(s), found %d",
							sbrparams.ExpectedReplicas, desired)
					}

					if liveDeploy.Object.Status.ReadyReplicas != sbrparams.ExpectedReplicas {
						return fmt.Errorf("expected %d ready replica(s), found %d",
							sbrparams.ExpectedReplicas, liveDeploy.Object.Status.ReadyReplicas)
					}

					sbrPods, listErr := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
					if listErr != nil {
						return listErr
					}

					runningPods := filterRunningPods(sbrPods)

					if len(runningPods) != int(sbrparams.ExpectedReplicas) {
						return fmt.Errorf("expected %d running SBR pod(s) for HA check, found %d",
							sbrparams.ExpectedReplicas, len(runningPods))
					}

					nodeNames := make(map[string]bool)

					for _, p := range runningPods {
						if p.Object.Spec.NodeName == "" {
							return fmt.Errorf("pod %s has not been assigned to a node", p.Object.Name)
						}

						nodeNames[p.Object.Spec.NodeName] = true
					}

					if len(nodeNames) != int(sbrparams.ExpectedReplicas) {
						return fmt.Errorf(
							"SBR pods must run on different nodes for HA, found pods on %d unique node(s)",
							len(nodeNames))
					}

					return nil
				}, medik8sparams.DefaultTimeout, 5*time.Second).Should(Succeed(),
					"SBR deployment did not stabilise at %d ready replicas on distinct nodes",
					sbrparams.ExpectedReplicas)
			})

		It("Verify SBR container runs as non-root user",
			reportxml.ID("89235"), func() {
				By("Getting SBR controller pod names")

				listOptions := metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s",
						sbrparams.OperatorControllerPodLabel),
				}

				var runningPods []*pod.Builder

				Eventually(func() error {
					sbrPods, listErr := pod.List(APIClient, medik8sparams.OperatorNs, listOptions)
					if listErr != nil {
						return listErr
					}

					running := filterRunningPods(sbrPods)
					if len(running) == 0 {
						return fmt.Errorf("no running SBR controller pods found")
					}

					runningPods = running

					return nil
				}, medik8sparams.DefaultTimeout, 5*time.Second).Should(Succeed(),
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

				sbrCSV := fetchActiveCSV()

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
		BeforeAll(func() {
			By("Cleaning up any leftover test SBRCs from previous runs")

			staleNames := []string{
				sbrparams.SBRCControllerTestName,
				sbrparams.SBRCWatchdogTestName,
				sbrparams.SBRCNoMatchSelectorTestName,
				fmt.Sprintf("%s-below-min-timeout", sbrparams.SBRCInvalidTestName),
				fmt.Sprintf("%s-above-max-timeout", sbrparams.SBRCInvalidTestName),
				fmt.Sprintf("%s-below-min-failures", sbrparams.SBRCInvalidTestName),
				fmt.Sprintf("%s-above-max-failures", sbrparams.SBRCInvalidTestName),
			}

			for _, name := range staleNames {
				staleRef := buildSBRC(name, map[string]interface{}{})

				deleteErr := APIClient.Delete(context.TODO(), staleRef)
				if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
					GinkgoT().Logf("Warning: pre-test cleanup of stale SBRC %s failed: %v", name, deleteErr)
				}
			}
		})

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

				var schemaErrors []string

				for _, invalidCase := range []invalidSBRCCase{
					{"below-min-timeout", "sbrTimeoutSeconds", sbrparams.SBRCTimeoutSecondsMin - 1},
					{"above-max-timeout", "sbrTimeoutSeconds", sbrparams.SBRCTimeoutSecondsMax + 1},
					{"below-min-failures", "maxConsecutiveFailures", sbrparams.SBRCMaxConsecutiveFailuresMin - 1},
					{"above-max-failures", "maxConsecutiveFailures", sbrparams.SBRCMaxConsecutiveFailuresMax + 1},
				} {
					By(fmt.Sprintf("Attempting to create SBRC with %s=%d (expect rejection)",
						invalidCase.field, invalidCase.value))

					invalidSBRC := buildSBRC(
						fmt.Sprintf("%s-%s", sbrparams.SBRCInvalidTestName, invalidCase.name),
						map[string]interface{}{invalidCase.field: invalidCase.value},
					)

					createErr := APIClient.Create(context.TODO(), invalidSBRC)
					if createErr == nil {
						invalidSBRCRef := invalidSBRC.DeepCopy()

						DeferCleanup(func() {
							deleteErr := APIClient.Delete(context.TODO(), invalidSBRCRef)
							if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
								GinkgoT().Logf("Warning: failed to delete unexpectedly-admitted SBRC %s: %v",
									invalidSBRCRef.GetName(), deleteErr)
							}
						})

						schemaErrors = append(schemaErrors,
							fmt.Sprintf("SBRC with %s=%d was unexpectedly admitted by the API server",
								invalidCase.field, invalidCase.value))

						continue
					}

					if !k8serrors.IsInvalid(createErr) && !k8serrors.IsBadRequest(createErr) {
						schemaErrors = append(schemaErrors,
							fmt.Sprintf("expected Invalid or BadRequest error for %s=%d, got: %v",
								invalidCase.field, invalidCase.value, createErr))
					}
				}

				if len(schemaErrors) > 0 {
					errMsg := "CRD schema validation failures:\n"
					for _, msg := range schemaErrors {
						errMsg += fmt.Sprintf("- %s\n", msg)
					}

					Fail(errMsg)
				}

				By("Layer 2: Controller validation — SBRC with non-existent StorageClass is admitted but DaemonSet is not deployed")

				By("Recording baseline DaemonSet names before creating the invalid SBRC")

				baselineDSList, listErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
					context.TODO(), metav1.ListOptions{})
				Expect(listErr).ToNot(HaveOccurred(), "Failed to list DaemonSets in operator namespace")

				baselineDSNames := make(map[string]bool, len(baselineDSList.Items))
				for _, ds := range baselineDSList.Items {
					baselineDSNames[ds.Name] = true
				}

				sbrc := buildSBRC(sbrparams.SBRCControllerTestName,
					map[string]interface{}{
						"sharedStorageClass": "nonexistent-storage-class",
					})

				err := APIClient.Create(context.TODO(), sbrc)
				Expect(err).ToNot(HaveOccurred(),
					"SBRC with invalid StorageClass reference should be admitted by API server")

				sbrcRef := sbrc.DeepCopy()

				DeferCleanup(func() {
					By("Cleaning up controller-layer test SBRC")

					deleteErr := APIClient.Delete(context.TODO(), sbrcRef)
					if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
						GinkgoT().Logf("Warning: failed to delete test SBRC %s: %v",
							sbrparams.SBRCControllerTestName, deleteErr)
					}
				})

				By("Verifying controller does not deploy a new DaemonSet for the invalid SBRC")

				Consistently(func() error {
					dsList, dsListErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
						context.TODO(), metav1.ListOptions{})
					if dsListErr != nil {
						return dsListErr
					}

					for _, daemonSet := range dsList.Items {
						if !baselineDSNames[daemonSet.Name] {
							return fmt.Errorf(
								"unexpected new DaemonSet %q appeared for SBRC with non-existent StorageClass",
								daemonSet.Name)
						}
					}

					return nil
				}, 30*time.Second, 5*time.Second).Should(Succeed(),
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

				baselineDSList, baselineErr := APIClient.DaemonSets(medik8sparams.OperatorNs).List(
					context.TODO(), metav1.ListOptions{})
				Expect(baselineErr).ToNot(HaveOccurred(), "Failed to list DaemonSets in operator namespace")

				baselineDSNames := make(map[string]bool, len(baselineDSList.Items))
				for _, ds := range baselineDSList.Items {
					baselineDSNames[ds.Name] = true
				}

				type invalidSBRCCase struct {
					name               string
					spec               map[string]interface{}
					desc               string
					requireNoDaemonSet bool
				}

				for _, invalidCase := range []invalidSBRCCase{
					{
						name: sbrparams.SBRCWatchdogTestName,
						spec: map[string]interface{}{
							"watchdogPath": sbrparams.SBRCInvalidWatchdogPath,
						},
						desc:               "invalid watchdog device path",
						requireNoDaemonSet: true,
					},
					{
						name: sbrparams.SBRCNoMatchSelectorTestName,
						spec: map[string]interface{}{
							"nodeSelector": map[string]interface{}{
								sbrparams.SBRCNoMatchSelectorKey: sbrparams.SBRCNoMatchSelectorValue,
							},
						},
						desc:               "nodeSelector matching no cluster nodes",
						requireNoDaemonSet: false,
					},
				} {
					By(fmt.Sprintf("Creating SBRC with %s", invalidCase.desc))

					sbrc := buildSBRC(invalidCase.name, invalidCase.spec)

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

							if invalidCase.requireNoDaemonSet {
								return fmt.Errorf("new DaemonSet %q must not exist for SBRC with %s",
									daemonSet.Name, invalidCase.desc)
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
