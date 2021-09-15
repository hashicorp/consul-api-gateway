package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/polar/internal/common/testlog"
)

type ControllerTestSuite struct {
	suite.Suite
	k8sClient  client.Client
	k8sManager ctrl.Manager
	testEnv    *envtest.Environment
	controller *Kubernetes
	consulSrv  *testutil.TestServer
}

func (suite *ControllerTestSuite) SetupTest() {
	suite.testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Scheme: scheme,
			Paths: []string{
				filepath.Join("..", "config", "crd", "bases"),
				filepath.Join("..", "config", "crd", "third-party", "gateway-api", "bases")},
			ErrorIfPathMissing: true,
			MaxTime:            0,
			PollInterval:       0,
			CleanUpAfterUse:    false,
		},
	}

	cfg, err := suite.testEnv.Start()
	suite.NoError(err)
	suite.NotNil(cfg)

	suite.k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	suite.NoError(err)
	suite.NotNil(suite.k8sClient)

	suite.k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	suite.NoError(err)
	suite.NotNil(suite.k8sManager)

	suite.controller, err = newWithManager(testlog.HCLogger(suite.T()), &Options{
		K8sRestConfig: cfg,
	}, suite.k8sManager)
	suite.NoError(err)
	suite.NotNil(suite.controller)

	suite.consulSrv, err = testutil.NewTestServerConfigT(suite.T(), func(c *testutil.TestServerConfig) {

	})
	suite.NoError(err)
	suite.NotNil(suite.consulSrv)
	suite.consulSrv.WaitForLeader(suite.T())
	suite.controller.SetConsul(suite.Consul())
	ok, _, err := suite.Consul().ConfigEntries().Set(&api.ProxyConfigEntry{
		Kind: api.ProxyDefaults,
		Name: "global",
		Config: map[string]interface{}{
			"protocol": "http",
		},
	}, nil)
	suite.Require().NoError(err)
	suite.Require().True(ok)
}

func (suite *ControllerTestSuite) TearDownTest() {
	suite.consulSrv.Stop()
	suite.testEnv.Stop()
}

func (suite *ControllerTestSuite) StartController(ctx context.Context) {
	go func() {
		err := suite.controller.Start(ctx)
		suite.NoError(err)
	}()
}

func (suite *ControllerTestSuite) Consul() *api.Client {
	cfg := api.DefaultConfig()
	cfg.Address = suite.consulSrv.HTTPAddr
	c, err := api.NewClient(cfg)
	suite.NoError(err)
	return c
}

func (suite *ControllerTestSuite) Client() client.Client {
	return suite.k8sClient
}

func TestControllerTestSuite(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip()
	}
	suite.Run(t, new(ControllerTestSuite))
}
