package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type SDSMetrics struct {
	ActiveStreams   prometheus.Gauge
	CachedResources prometheus.Gauge
}

type MetricsRegistry struct {
	SDS SDSMetrics
}

var Registry *MetricsRegistry

func init() {
	Registry = &MetricsRegistry{
		SDS: SDSMetrics{
			ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "sds_active_streams",
				Help: "The total number of active SDS streams",
			}),
			CachedResources: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "sds_cached_resources",
				Help: "The total number of resources in the certificate cache",
			}),
		},
	}
}
