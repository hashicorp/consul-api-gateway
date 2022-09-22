package builder

import (
	"fmt"
	"path/filepath"
	"strconv"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/hashicorp/consul-api-gateway/internal/version"
)

var (
	defaultImage              string
	defaultServiceAnnotations = []string{
		"external-dns.alpha.kubernetes.io/hostname",
	}
)

func init() {
	imageVersion := version.Version
	if version.VersionPrerelease != "" {
		imageVersion += "-" + version.VersionPrerelease
	}
	defaultImage = fmt.Sprintf("hashicorp/consul-api-gateway:%s", imageVersion)
}

const (
	defaultEnvoyImage           = "envoyproxy/envoy:v1.20-latest"
	defaultLogLevel             = "info"
	defaultConsulAddress        = "$(HOST_IP)"
	defaultConsulHTTPPort       = "8500"
	defaultConsulXDSPort        = "8502"
	defaultInstances      int32 = 1

	consulCALocalPath = "/consul/tls"
	consulCAFilename  = "ca.pem"

	k8sHostnameTopologyKey = "kubernetes.io/hostname"
)

var consulCALocalFile = filepath.Join(consulCALocalPath, consulCAFilename)

type Builder interface {
	Validate() error
}

type DeploymentBuilder interface {
	Builder
	Build(*int32) *v1.Deployment
}

type ServiceBuilder interface {
	Builder
	Build() *corev1.Service
}

func orDefault(value, defaultValue string) string {
	if value != "" {
		return value
	}
	return defaultValue
}

func orDefaultIntString(value int, defaultValue string) string {
	if value != 0 {
		return strconv.Itoa(value)
	}
	return defaultValue
}
