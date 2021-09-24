package reconciler

import (
	"sync"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

//go:generate mockgen -source ./tracker.go -destination ./mocks/tracker.go -package mocks GatewayStatusTracker

type podStatus struct {
	createdAt  meta.Time
	generation int64
	conditions []meta.Condition
}

func (p *podStatus) isUpdate(conditions []meta.Condition) bool {
	if len(conditions) != len(p.conditions) {
		// we have a different number of conditions, so they aren't the same
		return true
	}
	// this routine requires that the conditions are always in a stable order
	for i, newCondition := range conditions {
		oldCondition := p.conditions[i]
		if newCondition.Type != oldCondition.Type ||
			newCondition.Status != oldCondition.Status ||
			newCondition.Reason != oldCondition.Reason {
			return true
		}
	}
	return false
}

type GatewayStatusTracker interface {
	UpdateStatus(name types.NamespacedName, pod *core.Pod, conditions []meta.Condition, cb func() error) error
	DeleteStatus(name types.NamespacedName)
}

type StatusTracker struct {
	statuses map[types.NamespacedName]*podStatus
	mutex    sync.Mutex
}

func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		statuses: make(map[types.NamespacedName]*podStatus),
	}
}

// UpdateStatus calls the given callback if a pod status has been updated
// it does this so that it internally holds a synchronized mutex in order for
// updates to be consistent with the state of its internal cache.
func (p *StatusTracker) UpdateStatus(name types.NamespacedName, pod *core.Pod, conditions []meta.Condition, cb func() error) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	status, found := p.statuses[name]
	if !found {
		if err := cb(); err != nil {
			return err
		}
		p.statuses[name] = &podStatus{
			createdAt:  pod.CreationTimestamp,
			generation: pod.Generation,
			conditions: conditions,
		}
		return nil
	}
	if status.createdAt.After(pod.CreationTimestamp.Time) {
		// we have an old pod that's checking in, just ignore it
		return nil
	}
	// we only care about the current generation of pod updates or higher
	isCurrentGeneration := pod.Generation >= status.generation
	newerPod := pod.CreationTimestamp.After(status.createdAt.Time)
	if newerPod || (isCurrentGeneration && status.isUpdate(conditions)) {
		if err := cb(); err != nil {
			return err
		}
		status.createdAt = pod.CreationTimestamp
		status.generation = pod.Generation
		status.conditions = conditions
		return nil
	}
	// we have no update, just no-op
	return nil
}

func (p *StatusTracker) DeleteStatus(name types.NamespacedName) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.statuses, name)
}
