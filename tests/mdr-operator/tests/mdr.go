package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/medik8s/system-tests/tests/internal/medik8sinittools"
	"github.com/medik8s/system-tests/tests/internal/medik8sparams"
	"github.com/medik8s/system-tests/tests/mdr-operator/internal/mdrparams"
)

var _ = Describe(
	"MDR tests",
	Ordered,
	ContinueOnFailure,
	Label(mdrparams.Label), func() {
		BeforeAll(func() {
			By("Get MDR deployment object")

			mdrDeployment, err := deployment.Pull(
				APIClient, mdrparams.OperatorDeploymentName, medik8sparams.OperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get MDR deployment")

			By("Verify MDR deployment is Ready")
			Expect(mdrDeployment.IsReady(medik8sparams.DefaultTimeout)).To(BeTrue(), "MDR deployment is not Ready")
		})
		It("Verify Machine Deletion Remediation Operator pod is running", reportxml.ID("65767"), func() {
			_, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				medik8sparams.OperatorNs,
				medik8sparams.DefaultTimeout,
			)
			Expect(err).ToNot(HaveOccurred(), "Pod is not ready")
		})
	})
