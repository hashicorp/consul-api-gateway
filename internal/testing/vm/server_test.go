package vm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_TestController(t *testing.T) {
	controller := TestController(t)

	_, err := controller.Client.V1().Health(context.Background())
	require.NoError(t, err)

	name := "stub-deployment" // this should come from a created gateway.Name attribute
	token := "token"          // this should come from token minting
	controller.Deployment(t, name, token)

	// check gateway registration
	services, _, err := controller.Consul.Client.Catalog().Service(name, "", nil)
	require.NoError(t, err)
	require.Len(t, services, 1)
}
