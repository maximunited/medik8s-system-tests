package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/medik8s/system-tests/tests/internal/medik8sinittools"
	"github.com/medik8s/system-tests/tests/internal/medik8sparams"
	"github.com/medik8s/system-tests/tests/snr-operator/internal/snrparams"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildSNRCR builds an unstructured SNR custom resource of the given kind.
func buildSNRCR(kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": snrparams.CRDGroup + "/" + snrparams.CRDVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": medik8sparams.OperatorNs,
			},
		},
	}

	if spec != nil {
		resource.Object["spec"] = spec
	}

	return resource
}

// buildSNRWithAnnotations creates an SNR CR with optional annotations.
func buildSNRWithAnnotations(
	name string, annotations map[string]string,
) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": medik8sparams.OperatorNs,
	}

	if annotations != nil {
		annotationMap := make(map[string]interface{}, len(annotations))
		for key, val := range annotations {
			annotationMap[key] = val
		}

		metadata["annotations"] = annotationMap
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": snrparams.CRDGroup + "/" + snrparams.CRDVersion,
			"kind":       "SelfNodeRemediation",
			"metadata":   metadata,
		},
	}
}

// deferDeleteCR registers cleanup for a CR, waiting until the object is fully gone.
func deferDeleteCR(resource *unstructured.Unstructured) {
	DeferCleanup(func() {
		// Trigger deletion; ignore NotFound (already gone) and AlreadyExists-style no-ops.
		deleteErr := APIClient.Delete(context.TODO(), resource)
		if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) {
			GinkgoT().Logf("Warning: delete CR %q: %v", resource.GetName(), deleteErr)
		}

		// Wait until the object is actually gone — a finalizer can keep it in terminating
		// state after Delete returns nil, leaving stale objects visible to subsequent tests.
		Eventually(func() error {
			getErr := APIClient.Get(context.TODO(),
				client.ObjectKey{Name: resource.GetName(), Namespace: resource.GetNamespace()},
				resource)
			if k8serrors.IsNotFound(getErr) {
				return nil
			}

			if getErr != nil {
				return getErr
			}

			return fmt.Errorf("CR %q still exists (DeletionTimestamp: %v)",
				resource.GetName(), resource.GetDeletionTimestamp())
		}, medik8sparams.DefaultTimeout, snrparams.DefaultPollInterval).Should(Succeed(),
			"cleanup of test CR %q must complete", resource.GetName())
	})
}

// snrGVK returns the GVK for SelfNodeRemediation.
func snrGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   snrparams.CRDGroup,
		Version: snrparams.CRDVersion,
		Kind:    "SelfNodeRemediation",
	}
}
