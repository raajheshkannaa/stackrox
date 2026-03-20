package manager

import (
	"testing"

	"github.com/stackrox/rox/generated/internalapi/central"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/sensor/admission-control/resources"
	"github.com/stretchr/testify/assert"
)

func TestEnrichDeploymentWithNamespaceID(t *testing.T) {
	t.Run("successful namespace ID lookup", func(t *testing.T) {
		podStore := resources.NewPodStore()
		depStore := resources.NewDeploymentStore(podStore)
		nsStore := resources.NewNamespaceStore(depStore, podStore)

		// Add namespace to store
		namespace := &storage.NamespaceMetadata{
			Id:   "ns-123",
			Name: "test-namespace",
			Labels: map[string]string{
				"team": "backend",
			},
		}
		nsStore.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, namespace)

		mgr := &manager{
			namespaces: nsStore,
		}

		deployment := &storage.Deployment{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		}

		mgr.enrichDeploymentWithNamespaceID(deployment)

		assert.Equal(t, "ns-123", deployment.GetNamespaceId())
	})

	t.Run("namespace not found", func(t *testing.T) {
		podStore := resources.NewPodStore()
		depStore := resources.NewDeploymentStore(podStore)
		nsStore := resources.NewNamespaceStore(depStore, podStore)

		mgr := &manager{
			namespaces: nsStore,
		}

		deployment := &storage.Deployment{
			Name:      "test-deployment",
			Namespace: "nonexistent-namespace",
		}

		mgr.enrichDeploymentWithNamespaceID(deployment)

		// NamespaceId should remain empty when namespace not found
		assert.Empty(t, deployment.GetNamespaceId())
	})

	t.Run("deployment already has namespace ID", func(t *testing.T) {
		podStore := resources.NewPodStore()
		depStore := resources.NewDeploymentStore(podStore)
		nsStore := resources.NewNamespaceStore(depStore, podStore)

		// Add namespace to store
		namespace := &storage.NamespaceMetadata{
			Id:   "ns-123",
			Name: "test-namespace",
		}
		nsStore.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, namespace)

		mgr := &manager{
			namespaces: nsStore,
		}

		deployment := &storage.Deployment{
			Name:        "test-deployment",
			Namespace:   "test-namespace",
			NamespaceId: "existing-id",
		}

		mgr.enrichDeploymentWithNamespaceID(deployment)

		// Should overwrite existing ID with the correct one
		assert.Equal(t, "ns-123", deployment.GetNamespaceId())
	})
}
