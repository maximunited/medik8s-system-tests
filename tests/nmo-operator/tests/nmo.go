package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/medik8s/system-tests/tests/internal/medik8sinittools"
	"github.com/medik8s/system-tests/tests/internal/medik8sparams"
	"github.com/medik8s/system-tests/tests/nmo-operator/internal/nmoparams"
)

var _ = Describe(
	"NMO tests",
	Ordered,
	ContinueOnFailure,
	Label(nmoparams.Label), func() {
		BeforeAll(func() {
			By("Get NMO deployment object")

			nmoDeployment, err := deployment.Pull(
				APIClient, nmoparams.OperatorDeploymentName, medik8sparams.OperatorNs)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get NMO deployment %s", err))

			By("Verify NMO deployment is Ready")
			Expect(nmoDeployment.IsReady(medik8sparams.DefaultTimeout)).To(BeTrue(), "NMO deployment is not Ready")
		})
		It("Verify Node Maintenance Operator pod is running", reportxml.ID("46315"), func() {
			_, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				medik8sparams.OperatorNs,
				medik8sparams.DefaultTimeout,
			)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod is not ready %s", err))
		})
	})
