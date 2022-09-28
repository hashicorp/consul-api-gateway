package vm

import (
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
)

func Test_TestConsul(t *testing.T) {
	consul := TestConsul(t, true)

	_, err := consul.Client.KV().Put(&api.KVPair{
		Key:   "foo",
		Value: []byte("bar"),
	}, nil)
	require.NoError(t, err)

	kv, _, err := consul.Client.KV().Get("foo", nil)
	require.NoError(t, err)

	require.Equal(t, "bar", string(kv.Value))
}
