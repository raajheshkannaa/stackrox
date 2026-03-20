package resources

import (
	"github.com/stackrox/rox/generated/internalapi/central"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/sync"
)

// NewNamespaceStore returns new instance of NamespaceStore.
func NewNamespaceStore(deployments *DeploymentStore, pods *PodStore) *NamespaceStore {
	return &NamespaceStore{
		namespaces:       make(map[string]*storage.NamespaceMetadata),
		namespacesByName: make(map[string]*storage.NamespaceMetadata),
		deployments:      deployments,
		pods:             pods,
	}
}

// NamespaceStore stores namespace metadata and handles deployments and pod removal when namespace is removed.
type NamespaceStore struct {
	lock             sync.RWMutex
	namespaces       map[string]*storage.NamespaceMetadata // keyed by namespace ID
	namespacesByName map[string]*storage.NamespaceMetadata // keyed by namespace name
	deployments      *DeploymentStore
	pods             *PodStore
}

// ProcessEvent processes namespace event.
func (m *NamespaceStore) ProcessEvent(action central.ResourceAction, obj interface{}) {
	ns, isNs := obj.(*storage.NamespaceMetadata)
	if !isNs {
		return
	}

	switch action {
	case central.ResourceAction_CREATE_RESOURCE, central.ResourceAction_UPDATE_RESOURCE, central.ResourceAction_SYNC_RESOURCE:
		m.lock.Lock()
		defer m.lock.Unlock()
		m.namespaces[ns.GetId()] = ns
		m.namespacesByName[ns.GetName()] = ns
	case central.ResourceAction_REMOVE_RESOURCE:
		// Namespace remove event contains full namespace metadata.
		m.lock.Lock()
		defer m.lock.Unlock()
		delete(m.namespaces, ns.GetId())
		delete(m.namespacesByName, ns.GetName())
		m.deployments.OnNamespaceDelete(ns.GetName())
		m.pods.OnNamespaceDelete(ns.GetName())
	}
}

// LookupNamespaceLabelsByID returns the labels for the given namespace ID.
func (m *NamespaceStore) LookupNamespaceLabelsByID(id string) (map[string]string, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	metadata, found := m.namespaces[id]
	if !found {
		return nil, false
	}
	return metadata.GetLabels(), true
}

// LookupNamespaceIDByName returns the namespace ID for the given namespace name.
func (m *NamespaceStore) LookupNamespaceIDByName(name string) (string, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	metadata, found := m.namespacesByName[name]
	if !found {
		return "", false
	}
	return metadata.GetId(), true
}
