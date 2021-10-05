package common

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
)

func Test_ServiceNamesIndex(t *testing.T) {
	require := require.New(t)
	require.NotNil(NewServiceNameIndex())

	idx1 := NewServiceNameIndex()
	idx2 := NewServiceNameIndex()

	n := func(name, namespace string) api.CompoundServiceName {
		return api.CompoundServiceName{Name: name, Namespace: namespace}
	}

	idx1.Add(n("name1", ""), n("name2", ""), n("name1", "namespace1"))
	require.Len(idx1.idx, 3)
	idx1.Add(n("name1", "namespace1"), n("name2", "namespace2"))
	require.Len(idx1.idx, 4)
	idx1.Remove(n("name1", ""), n("name1", ""))
	require.Len(idx1.idx, 3)
	require.False(idx1.Exists(n("name1", "")))
	require.True(idx1.Exists(n("name2", "")))

	idx2.Add(n("name1", ""), n("name1", "namespace1"))
	added, removed := idx1.Diff(idx2)
	require.Len(added, 1)
	require.Contains(added, n("name1", ""))
	require.Len(removed, 2)
	require.Contains(removed, n("name2", ""))
	require.Contains(removed, n("name2", "namespace2"))
}
