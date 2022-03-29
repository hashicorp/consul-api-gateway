//go:build conformance
// +build conformance

package conformance

import (
	"os"
	"testing"

	"github.com/hashicorp/consul-api-gateway/internal/testing/e2e"
	"sigs.k8s.io/e2e-framework/pkg/env"
)

//This file is intended to help facilitate an easy and local way of running the
//conformance tests defined here https://github.com/kubernetes-sigs/gateway-api/blob/master/conformance/conformance_test.go

//This is a work in progress

var (
	testenv   env.Environment
	hostRoute string
)

func TestMain(m *testing.M) {
	//set up test env
	hostRoute := os.Getenv("DOCKER_HOST_ROUTE")
	if hostRoute == "" {
		hostRoute = "host.docker.internal"
	}

	testenv = env.New()
	testenv.Setup(
		SetUpStack(hostRoute),
	)

	testenv.Finish(
		e2e.TearDownStack,
	)

	testenv.Run(m)
}

func TestRunConformanceTestSuite(t *testing.T) {
	//TODO wait for gateway to be ready

	//TODO invoke conformance tests

}
