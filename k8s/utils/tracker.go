package utils

import (
	"fmt"
	"sync"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

type PodTracker struct {
	statuses map[string]*podStatus
	mutex    sync.Mutex
}

func NewPodTracker() *PodTracker {
	return &PodTracker{
		statuses: make(map[string]*podStatus),
	}
}

func (p *PodTracker) UpdateStatus(pod *core.Pod, conditions []meta.Condition) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	key := podKey(pod)
	status, found := p.statuses[key]
	if !found {
		p.statuses[key] = &podStatus{
			createdAt:  pod.CreationTimestamp,
			generation: pod.Generation,
			conditions: conditions,
		}
		return true
	}
	if status.createdAt.After(pod.CreationTimestamp.Time) {
		// we have an old pod that's checking in, just ignore it
		return false
	}
	if status.generation > pod.Generation {
		// we already have a newer status, ignore
		return false
	}
	if status.isUpdate(conditions) {
		status.createdAt = pod.CreationTimestamp
		status.generation = pod.Generation
		status.conditions = conditions
		return true
	}
	return false
}

func (p *PodTracker) DeleteStatus(pod *core.Pod) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	key := podKey(pod)
	status, found := p.statuses[key]
	if !found {
		return
	}
	if status.createdAt.After(pod.CreationTimestamp.Time) {
		// we have an old pod that's being deleted, just ignore it
		return
	}
	delete(p.statuses, key)
}

func podKey(pod *core.Pod) string {
	return fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
}
