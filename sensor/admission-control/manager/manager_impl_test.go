package manager

import (
	"testing"
	"time"

	"github.com/stackrox/rox/generated/internalapi/central"
	"github.com/stackrox/rox/generated/internalapi/sensor"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/concurrency"
	"github.com/stackrox/rox/pkg/features"
	"github.com/stackrox/rox/pkg/protocompat"
	"github.com/stackrox/rox/sensor/admission-control/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessNewSettings_LabelProvidersWiring(t *testing.T) {
	// Enable the label-based policy scoping feature flag for all tests
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("cluster label provider correctly wired from settings", func(t *testing.T) {
		mgr := createTestManager()

		clusterLabels := map[string]string{
			"env":    "prod",
			"region": "us-east-1",
		}

		settings := createTestSettings(
			withClusterLabels(clusterLabels),
			withRuntimePolicy(createPolicyWithClusterLabelScope("cluster-scoped-policy", "env", "prod")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		// Verify cluster label provider was wired by checking policy set compilation
		assert.NotNil(t, state.allK8sEventDetector)
		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 1)
		// Verify the policy exists in the map
		for _, policy := range policies {
			assert.Equal(t, "cluster-scoped-policy", policy.Policy().GetName())
		}
	})

	t.Run("namespace label provider correctly wired from namespace store", func(t *testing.T) {
		mgr := createTestManager()

		// Add namespace to store
		namespace := &storage.NamespaceMetadata{
			Id:   "ns-123",
			Name: "backend",
			Labels: map[string]string{
				"team": "backend",
				"tier": "app",
			},
		}
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, namespace) // CREATE_RESOURCE

		settings := createTestSettings(
			withRuntimePolicy(createPolicyWithNamespaceLabelScope("namespace-scoped-policy", "team", "backend")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		// Verify namespace label provider was wired
		assert.NotNil(t, state.allK8sEventDetector)
		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 1)
	})

	t.Run("both cluster and namespace label providers wired together", func(t *testing.T) {
		mgr := createTestManager()

		namespace := &storage.NamespaceMetadata{
			Id:   "ns-456",
			Name: "frontend",
			Labels: map[string]string{
				"team": "frontend",
			},
		}
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, namespace)

		settings := createTestSettings(
			withClusterLabels(map[string]string{"env": "staging"}),
			withRuntimePolicy(createPolicyWithClusterLabelScope("cluster-policy", "env", "staging")),
			withRuntimePolicy(createPolicyWithNamespaceLabelScope("namespace-policy", "team", "frontend")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 2)
	})

	t.Run("deploy-time policies with label providers", func(t *testing.T) {
		mgr := createTestManager()

		namespace := &storage.NamespaceMetadata{
			Id:   "ns-789",
			Name: "production",
			Labels: map[string]string{
				"env": "production",
			},
		}
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, namespace)

		settings := createTestSettings(
			withClusterLabels(map[string]string{"cloud": "aws"}),
			withDeployTimePolicy(createEnforcedPolicyWithClusterLabelScope("deploy-cluster-policy", "cloud", "aws")),
			withDeployTimePolicy(createEnforcedPolicyWithNamespaceLabelScope("deploy-namespace-policy", "env", "production")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		// Check spec-only detector policies
		specPolicies := state.specOnlyDeployDetector.PolicySet().GetCompiledPolicies()
		enrichmentPolicies := state.enrichmentRequiredDeployDetector.PolicySet().GetCompiledPolicies()

		totalPolicies := len(specPolicies) + len(enrichmentPolicies)
		assert.Equal(t, 2, totalPolicies, "Both deploy-time policies should be compiled")
	})
}

func TestProcessNewSettings_EmptyClusterLabels(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("empty cluster labels map", func(t *testing.T) {
		mgr := createTestManager()

		settings := createTestSettings(
			withClusterLabels(map[string]string{}),
			withRuntimePolicy(createPolicyWithClusterLabelScope("policy", "env", "prod")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)
		assert.NotNil(t, state.allK8sEventDetector)
	})

	t.Run("nil cluster labels", func(t *testing.T) {
		mgr := createTestManager()

		settings := createTestSettings(
			withRuntimePolicy(createPolicyWithClusterLabelScope("policy", "env", "prod")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)
		assert.NotNil(t, state.allK8sEventDetector)
	})
}

func TestProcessNewSettings_NamespaceNotInStore(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("namespace label scoped policy when namespace not in store", func(t *testing.T) {
		mgr := createTestManager()

		// Don't add namespace to store
		settings := createTestSettings(
			withRuntimePolicy(createPolicyWithNamespaceLabelScope("policy", "team", "backend")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		// Policy should still be compiled even if namespace doesn't exist yet
		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 1)
	})
}

func TestProcessNewSettings_ClusterLabelsUpdate(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("cluster labels updated between settings", func(t *testing.T) {
		mgr := createTestManager()

		// Initial settings with env=dev
		settings1 := createTestSettings(
			withClusterLabels(map[string]string{"env": "dev"}),
			withRuntimePolicy(createPolicyWithClusterLabelScope("policy", "env", "dev")),
		)
		mgr.ProcessNewSettings(settings1)

		state1 := mgr.currentState()
		require.NotNil(t, state1)

		// Updated settings with env=prod
		settings2 := createTestSettings(
			withClusterLabels(map[string]string{"env": "prod"}),
			withRuntimePolicy(createPolicyWithClusterLabelScope("policy", "env", "prod")),
		)
		settings2.Timestamp = protocompat.TimestampNow()
		time.Sleep(time.Millisecond) // Ensure timestamp is newer
		mgr.ProcessNewSettings(settings2)

		state2 := mgr.currentState()
		require.NotNil(t, state2)
		assert.NotSame(t, state1, state2, "State should be updated")
	})
}

func TestProcessNewSettings_MultipleNamespaces(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("policies scoped to multiple namespaces", func(t *testing.T) {
		mgr := createTestManager()

		// Add multiple namespaces
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, &storage.NamespaceMetadata{
			Id:     "ns-1",
			Name:   "namespace-1",
			Labels: map[string]string{"tier": "frontend"},
		})
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, &storage.NamespaceMetadata{
			Id:     "ns-2",
			Name:   "namespace-2",
			Labels: map[string]string{"tier": "backend"},
		})
		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, &storage.NamespaceMetadata{
			Id:     "ns-3",
			Name:   "namespace-3",
			Labels: map[string]string{"tier": "database"},
		})

		settings := createTestSettings(
			withRuntimePolicy(createPolicyWithNamespaceLabelScope("frontend-policy", "tier", "frontend")),
			withRuntimePolicy(createPolicyWithNamespaceLabelScope("backend-policy", "tier", "backend")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 2)
	})
}

func TestProcessNewSettings_CombinedClusterAndNamespaceScoping(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("policy with both cluster and namespace label scopes", func(t *testing.T) {
		mgr := createTestManager()

		mgr.namespaces.ProcessEvent(central.ResourceAction_CREATE_RESOURCE, &storage.NamespaceMetadata{
			Id:     "ns-prod",
			Name:   "production",
			Labels: map[string]string{"env": "production"},
		})

		policy := createPolicyWithCombinedScope("combined-policy", "cloud", "aws", "env", "production")

		settings := createTestSettings(
			withClusterLabels(map[string]string{"cloud": "aws", "region": "us-east-1"}),
			withRuntimePolicy(policy),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		policies := state.allK8sEventDetector.PolicySet().GetCompiledPolicies()
		assert.Len(t, policies, 1)
		for _, policy := range policies {
			assert.Equal(t, "combined-policy", policy.Policy().GetName())
		}
	})
}

func TestProcessNewSettings_SpecOnlyVsEnrichmentRequired(t *testing.T) {
	t.Setenv(features.LabelBasedPolicyScoping.EnvVar(), "true")

	t.Run("label-scoped policies split between spec-only and enrichment-required", func(t *testing.T) {
		mgr := createTestManager()

		settings := createTestSettings(
			withClusterLabels(map[string]string{"env": "prod"}),
			// Spec-only policy (deployment metadata only)
			withDeployTimePolicy(createEnforcedPolicyWithClusterLabelScope("spec-only", "env", "prod")),
			// Enrichment-required policy (requires image scan data)
			withDeployTimePolicy(createEnforcedImagePolicyWithClusterLabelScope("image-required", "env", "prod")),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)

		specPolicies := state.specOnlyDeployDetector.PolicySet().GetCompiledPolicies()
		enrichmentPolicies := state.enrichmentRequiredDeployDetector.PolicySet().GetCompiledPolicies()

		assert.Greater(t, len(specPolicies)+len(enrichmentPolicies), 0, "At least one policy should be compiled")
	})
}

func TestProcessNewSettings_NilSettings(t *testing.T) {
	t.Run("nil settings disables admission control", func(t *testing.T) {
		mgr := createTestManager()

		// First set valid settings
		settings := createTestSettings(
			withClusterLabels(map[string]string{"env": "dev"}),
		)
		mgr.ProcessNewSettings(settings)
		require.NotNil(t, mgr.currentState())

		// Then process nil settings
		mgr.ProcessNewSettings(nil)

		assert.Nil(t, mgr.currentState())
	})
}

func TestProcessNewSettings_NoLabelScopedPolicies(t *testing.T) {
	t.Run("settings with no label-scoped policies", func(t *testing.T) {
		mgr := createTestManager()

		policy := &storage.Policy{
			Id:   "policy-1",
			Name: "no-label-scope",
			PolicySections: []*storage.PolicySection{
				{
					PolicyGroups: []*storage.PolicyGroup{
						{
							FieldName: "Privileged Container",
							Values: []*storage.PolicyValue{
								{Value: "true"},
							},
						},
					},
				},
			},
			EventSource: storage.EventSource_DEPLOYMENT_EVENT,
			LifecycleStages: []storage.LifecycleStage{
				storage.LifecycleStage_RUNTIME,
			},
		}

		settings := createTestSettings(
			withClusterLabels(map[string]string{"env": "prod"}),
			withRuntimePolicy(policy),
		)

		mgr.ProcessNewSettings(settings)

		state := mgr.currentState()
		require.NotNil(t, state)
		assert.NotNil(t, state.allK8sEventDetector)
	})
}

// Helper functions

func createTestManager() *manager {
	podStore := resources.NewPodStore()
	depStore := resources.NewDeploymentStore(podStore)
	nsStore := resources.NewNamespaceStore(depStore, podStore)

	return &manager{
		namespaces:     nsStore,
		ownNamespace:   "stackrox",
		settingsStream: concurrency.NewValueStream[*sensor.AdmissionControlSettings](nil),
	}
}

type settingsOption func(*sensor.AdmissionControlSettings)

func withClusterLabels(labels map[string]string) settingsOption {
	return func(s *sensor.AdmissionControlSettings) {
		s.ClusterLabels = &sensor.ClusterLabels{Labels: labels}
	}
}

func withRuntimePolicy(policy *storage.Policy) settingsOption {
	return func(s *sensor.AdmissionControlSettings) {
		if s.GetRuntimePolicies() == nil {
			s.RuntimePolicies = &storage.PolicyList{}
		}
		s.RuntimePolicies.Policies = append(s.GetRuntimePolicies().GetPolicies(), policy)
	}
}

func withDeployTimePolicy(policy *storage.Policy) settingsOption {
	return func(s *sensor.AdmissionControlSettings) {
		if s.GetEnforcedDeployTimePolicies() == nil {
			s.EnforcedDeployTimePolicies = &storage.PolicyList{}
		}
		s.EnforcedDeployTimePolicies.Policies = append(s.GetEnforcedDeployTimePolicies().GetPolicies(), policy)
	}
}

func createTestSettings(opts ...settingsOption) *sensor.AdmissionControlSettings {
	settings := &sensor.AdmissionControlSettings{
		ClusterConfig: &storage.DynamicClusterConfig{
			AdmissionControllerConfig: &storage.AdmissionControllerConfig{
				Enabled: true,
			},
		},
		ClusterId:                  "test-cluster",
		Timestamp:                  protocompat.TimestampNow(),
		EnforcedDeployTimePolicies: &storage.PolicyList{},
		RuntimePolicies:            &storage.PolicyList{},
	}

	for _, opt := range opts {
		opt(settings)
	}

	return settings
}

func createPolicyWithClusterLabelScope(name, labelKey, labelValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				ClusterLabel: &storage.Scope_Label{
					Key:   labelKey,
					Value: labelValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "Kubernetes Resource",
						Values: []*storage.PolicyValue{
							{Value: "PODS_EXEC"},
						},
					},
				},
			},
		},
		EventSource: storage.EventSource_DEPLOYMENT_EVENT,
		LifecycleStages: []storage.LifecycleStage{
			storage.LifecycleStage_RUNTIME,
		},
		PolicyVersion: "1.1",
	}
}

func createPolicyWithNamespaceLabelScope(name, labelKey, labelValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				NamespaceLabel: &storage.Scope_Label{
					Key:   labelKey,
					Value: labelValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "Kubernetes Resource",
						Values: []*storage.PolicyValue{
							{Value: "PODS_PORTFORWARD"},
						},
					},
				},
			},
		},
		EventSource: storage.EventSource_DEPLOYMENT_EVENT,
		LifecycleStages: []storage.LifecycleStage{
			storage.LifecycleStage_RUNTIME,
		},
		PolicyVersion: "1.1",
	}
}

func createPolicyWithCombinedScope(name, clusterKey, clusterValue, namespaceKey, namespaceValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				ClusterLabel: &storage.Scope_Label{
					Key:   clusterKey,
					Value: clusterValue,
				},
				NamespaceLabel: &storage.Scope_Label{
					Key:   namespaceKey,
					Value: namespaceValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "Kubernetes Resource",
						Values: []*storage.PolicyValue{
							{Value: "PODS_EXEC"},
						},
					},
				},
			},
		},
		EventSource: storage.EventSource_DEPLOYMENT_EVENT,
		LifecycleStages: []storage.LifecycleStage{
			storage.LifecycleStage_RUNTIME,
		},
		PolicyVersion: "1.1",
	}
}

func createEnforcedPolicyWithClusterLabelScope(name, labelKey, labelValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				ClusterLabel: &storage.Scope_Label{
					Key:   labelKey,
					Value: labelValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "Privileged Container",
						Values: []*storage.PolicyValue{
							{Value: "true"},
						},
					},
				},
			},
		},
		LifecycleStages: []storage.LifecycleStage{storage.LifecycleStage_DEPLOY},
		EnforcementActions: []storage.EnforcementAction{
			storage.EnforcementAction_SCALE_TO_ZERO_ENFORCEMENT,
		},
		PolicyVersion: "1.1",
	}
}

func createEnforcedPolicyWithNamespaceLabelScope(name, labelKey, labelValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				NamespaceLabel: &storage.Scope_Label{
					Key:   labelKey,
					Value: labelValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "Privileged Container",
						Values: []*storage.PolicyValue{
							{Value: "true"},
						},
					},
				},
			},
		},
		LifecycleStages: []storage.LifecycleStage{storage.LifecycleStage_DEPLOY},
		EnforcementActions: []storage.EnforcementAction{
			storage.EnforcementAction_SCALE_TO_ZERO_ENFORCEMENT,
		},
		PolicyVersion: "1.1",
	}
}

func createEnforcedImagePolicyWithClusterLabelScope(name, labelKey, labelValue string) *storage.Policy {
	return &storage.Policy{
		Id:   name + "-id",
		Name: name,
		Scope: []*storage.Scope{
			{
				ClusterLabel: &storage.Scope_Label{
					Key:   labelKey,
					Value: labelValue,
				},
			},
		},
		PolicySections: []*storage.PolicySection{
			{
				PolicyGroups: []*storage.PolicyGroup{
					{
						FieldName: "CVE",
						Values: []*storage.PolicyValue{
							{Value: "CVE-2021-44228"},
						},
					},
				},
			},
		},
		LifecycleStages: []storage.LifecycleStage{storage.LifecycleStage_DEPLOY},
		EnforcementActions: []storage.EnforcementAction{
			storage.EnforcementAction_SCALE_TO_ZERO_ENFORCEMENT,
		},
		PolicyVersion: "1.1",
	}
}
