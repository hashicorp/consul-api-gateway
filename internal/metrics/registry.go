package metrics

import (
	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
)

var (
	SDSActiveStreams             = []string{"sds_active_streams"}
	SDSCachedResources           = []string{"sds_cached_resources"}
	SDSCertificateFetches        = []string{"sds_certificate_fetches"}
	K8sGateways                  = []string{"k8s_gateways"}
	K8sNewGatewayDeployments     = []string{"k8s_new_gateway_deployments"}
	ConsulLeafCertificateFetches = []string{"consul_leaf_certificate_fetches"}
)

var Registry metrics.MetricSink

func init() {
	sink, err := prometheus.NewPrometheusSinkFrom(prometheus.PrometheusOpts{
		GaugeDefinitions: []prometheus.GaugeDefinition{{
			Name: SDSActiveStreams,
			Help: "The total number of active SDS streams",
		}, {
			Name: SDSCachedResources,
			Help: "The total number of resources in the certificate cache",
		}, {
			Name: K8sGateways,
			Help: "The number of gateways the kubernetes controller is tracking",
		}},
		CounterDefinitions: []prometheus.CounterDefinition{{
			Name: SDSCertificateFetches,
			Help: "The total number of fetches per certificate segmented by fetcher",
		}, {
			Name: K8sNewGatewayDeployments,
			Help: "The number of gateways the kubernetes controller has deployed",
		}, {
			Name: ConsulLeafCertificateFetches,
			Help: "The number of times a leaf certificate has been fetched from Consul",
		}},
	})
	if err != nil {
		panic(err)
	}
	Registry = sink
}
