package common

import (
	"github.com/hashicorp/consul/api"
)

type ServiceNameIndex struct {
	idx map[api.CompoundServiceName]struct{}
}

func NewServiceNameIndex() *ServiceNameIndex {
	return &ServiceNameIndex{idx: map[api.CompoundServiceName]struct{}{}}
}

func (i *ServiceNameIndex) Exists(name api.CompoundServiceName) bool {
	_, ok := i.idx[name]
	return ok
}

func (i *ServiceNameIndex) Add(names ...api.CompoundServiceName) {
	for _, name := range names {
		i.idx[name] = struct{}{}
	}
}

func (i *ServiceNameIndex) Remove(names ...api.CompoundServiceName) {
	for _, name := range names {
		delete(i.idx, name)
	}
}

func (i *ServiceNameIndex) Diff(other *ServiceNameIndex) (added []api.CompoundServiceName, removed []api.CompoundServiceName) {
	for name := range other.idx {
		if _, ok := i.idx[name]; !ok {
			// other has added name
			added = append(added, name)
		}
	}

	for name := range i.idx {
		if _, ok := other.idx[name]; !ok {
			// other is removing name
			removed = append(removed, name)
		}
	}

	return added, removed
}

func (i *ServiceNameIndex) All() []api.CompoundServiceName {
	var result []api.CompoundServiceName
	for name := range i.idx {
		result = append(result, name)
	}
	return result
}
