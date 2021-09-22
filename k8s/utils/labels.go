package utils

import (
	"k8s.io/apimachinery/pkg/types"
	gateway "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	ManagedLabel   = "api-gateway.consul.hashicorp.com/managed"
	nameLabel      = "api-gateway.consul.hashicorp.com/name"
	namespaceLabel = "api-gateway.consul.hashicorp.com/namespace"
)

func LabelsForNamedGateway(name types.NamespacedName) map[string]string {
	return map[string]string{
		nameLabel:      name.Name,
		namespaceLabel: name.Namespace,
		ManagedLabel:   "true",
	}
}

func LabelsForGateway(gw *gateway.Gateway) map[string]string {
	return LabelsForNamedGateway(KubeObjectNamespacedName(gw))
}

func IsManagedGateway(labels map[string]string) (string, bool) {
	managed, ok := labels[ManagedLabel]

	if !ok || managed != "true" {
		return "", false
	}
	name, ok := labels[nameLabel]
	if !ok {
		return "", false
	}
	return name, true
}
