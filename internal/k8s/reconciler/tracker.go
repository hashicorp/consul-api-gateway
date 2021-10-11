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

// GatewayStatusTracker is leveraged to track gateway status updates
// based on the status of an underlying deployed pod.
type GatewayStatusTracker interface {
	// UpdateStatus should call the given callback if a pod status has been updated.
	UpdateStatus(name types.NamespacedName, pod *core.Pod, conditions []meta.Condition, force bool, cb func() error) error
	// DeleteStatus cleans up the status tracking for the given gateway
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
// updates to be consistent with the state of its internal cache. Any errors
// returned come from the callback.
func (p *StatusTracker) UpdateStatus(name types.NamespacedName, pod *core.Pod, conditions []meta.Condition, force bool, cb func() error) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if pod == nil {
		// we have no pod, don't cache anything
		if force {
			return cb()
		}
		return nil
	}

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
	} else if force {
		return cb()
	}
	// we have no update, just no-op
	return nil
}

// DeleteStatus cleans up the status tracking for the given gateway
func (p *StatusTracker) DeleteStatus(name types.NamespacedName) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.statuses, name)
}
