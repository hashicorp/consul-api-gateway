package reconciler

import (
	"context"
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

func testIntentionsReconciler(t *testing.T, disco consulDiscoveryChains, config consulConfigEntries) *IntentionsReconciler {
	r := &IntentionsReconciler{
		consulDisco:      disco,
		consulConfig:     config,
		serviceName:      api.CompoundServiceName{Name: "name1", Namespace: "namespace1"},
		ctx:              context.Background(),
		targetIndex:      common.NewServiceNameIndex(),
		targetTombstones: common.NewServiceNameIndex(),
		logger:           testutil.Logger(t),
	}
	return r
}
