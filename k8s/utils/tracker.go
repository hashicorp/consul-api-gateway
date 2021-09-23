package utils

import (
	"sync"
	"time"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type podStatus struct {
	createdAt  time.Time
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

type StatusTracker struct {
	statuses map[types.NamespacedName]*podStatus
	mutex    sync.Mutex
}

func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		statuses: make(map[types.NamespacedName]*podStatus),
	}
}

func (p *StatusTracker) UpdateStatus(name types.NamespacedName, generation int64, createdAt time.Time, conditions []meta.Condition) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	status, found := p.statuses[name]
	if !found {
		p.statuses[name] = &podStatus{
			createdAt:  createdAt,
			generation: generation,
			conditions: conditions,
		}
		return true
	}
	if status.createdAt.After(createdAt) {
		// we have an old pod that's checking in, just ignore it
		return false
	}
	if status.generation > generation {
		// we already have a newer status, ignore
		return false
	}
	newerPod := createdAt.After(status.createdAt)
	if newerPod || status.isUpdate(conditions) {
		status.createdAt = createdAt
		status.generation = generation
		status.conditions = conditions
		return true
	}
	return false
}

func (p *StatusTracker) DeleteStatus(name types.NamespacedName) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.statuses, name)
}
