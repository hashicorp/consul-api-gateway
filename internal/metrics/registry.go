package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type SDSMetrics struct {
	ActiveStreams      prometheus.Gauge
	CachedResources    prometheus.Gauge
	CertificateFetches prometheus.CounterVec
}

type K8sMetrics struct {
	Gateways              prometheus.Gauge
	NewGatewayDeployments prometheus.Counter
}

type MetricsRegistry struct {
	SDS *SDSMetrics
	K8s *K8sMetrics
}

var Registry *MetricsRegistry

func init() {
	Registry = &MetricsRegistry{
		SDS: &SDSMetrics{
			ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "sds_active_streams",
				Help: "The total number of active SDS streams",
			}),
			CachedResources: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "sds_cached_resources",
				Help: "The total number of resources in the certificate cache",
			}),
			CertificateFetches: *promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "sds_certificate_fetches",
				Help: "The total number of fetches per certificate",
			}, []string{"name", "fetcher"}),
		},
		K8s: &K8sMetrics{
			Gateways: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "k8s_gateways",
				Help: "The number of gateways the kubernetes controller is tracking",
			}),
			NewGatewayDeployments: promauto.NewCounter(prometheus.CounterOpts{
				Name: "k8s_new_gateway_deployments",
				Help: "The number of gateways the kubernetes controller has deployed",
			}),
		},
	}
}
