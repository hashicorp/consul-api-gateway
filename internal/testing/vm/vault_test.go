package vm

import (
	"context"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/require"
)

func Test_TestVault(t *testing.T) {
	vault := TestVault(t)

	require.NoError(t, vault.Client.Sys().Mount("mount", &api.MountInput{Type: "kv-v2"}))

	_, err := vault.Client.KVv2("mount").Put(context.Background(), "foo", map[string]interface{}{
		"bar": "baz",
	})
	require.NoError(t, err)

	secret, err := vault.Client.KVv2("mount").Get(context.Background(), "foo")
	require.NoError(t, err)

	require.Equal(t, "baz", secret.Data["bar"])
}
