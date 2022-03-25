package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/hashicorp/consul-api-gateway/internal/core"
	"github.com/hashicorp/consul-api-gateway/internal/store"
	"github.com/hashicorp/go-hclog"
)

func TestListenerID(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
	require.Equal(t, "test", NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Name: gw.SectionName("test"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).ID())
}

func TestListenerConfig(t *testing.T) {
	t.Parallel()

	require.Equal(t, store.ListenerConfig{
		Name: "listener",
		TLS:  core.TLSParams{Enabled: false},
	}, NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Name: gw.SectionName("listener"),
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())

	hostname := gw.Hostname("hostname")
	require.Equal(t, store.ListenerConfig{
		Name:     "default",
		Hostname: "hostname",
		TLS:      core.TLSParams{Enabled: false},
	}, NewK8sListener(&K8sGateway{Gateway: &gw.Gateway{}}, gw.Listener{
		Hostname: &hostname,
	}, K8sListenerConfig{
		Logger: hclog.NewNullLogger(),
	}).Config())
}
