// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/hashicorp/consul/api"
	capi "github.com/hashicorp/consul/api"
)

const (
	WildcardNamespace = "*"
	DefaultNamespace  = "default"
)

type PartitionInfo struct {
	EnablePartitions bool
	PartitionName    string
}

// EnsureNamespaceExists ensures a Consul namespace with name ns exists. If it doesn't,
// it will create it and set crossNSACLPolicy as a policy default.
// Boolean return value indicates if the namespace was created by this call.
func EnsureNamespaceExists(client Client, ns string, partitionInfo PartitionInfo) (bool, error) {
	if ns == WildcardNamespace || ns == DefaultNamespace {
		return false, nil
	}

	// Check if the Consul namespace exists.
	namespaceInfo, _, err := client.Namespaces().Read(ns, nil)
	if err != nil {
		return false, err
	}
	if namespaceInfo != nil {
		return false, nil
	}

	// If the namespace does not, create it with default cross-namespace-policy.
	crossNamespacePolicy, err := getOrCreateCrossNamespacePolicy(client, partitionInfo)
	if err != nil {
		return false, err
	}

	aclConfig := capi.NamespaceACLConfig{
		PolicyDefaults: []api.ACLLink{
			{Name: crossNamespacePolicy.Name},
		},
	}

	consulNamespace := capi.Namespace{
		Name:        ns,
		Description: "Auto-generated by consul-api-gateway",
		ACLs:        &aclConfig,
		Meta:        map[string]string{"external-source": "kubernetes"},
	}

	_, _, err = client.Namespaces().Create(&consulNamespace, nil)
	if err != nil {
		return false, err
	}
	return true, err
}

func getOrCreateCrossNamespacePolicy(client Client, partitionInfo PartitionInfo) (*api.ACLPolicy, error) {
	acl := client.ACL()
	policyName := "cross-namespace-policy"
	policy, _, err := acl.PolicyReadByName(policyName, nil)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		return policy, nil
	}
	rules, err := crossNamespaceRules(partitionInfo)
	if err != nil {
		return &api.ACLPolicy{}, err
	}
	policy = &api.ACLPolicy{
		Name:        policyName,
		Description: "Policy to allow permissions to cross Consul namespaces for k8s services",
		Rules:       rules,
	}
	createdPolicy, _, err := acl.PolicyCreate(policy, nil)
	if err != nil {
		return nil, err
	}
	return createdPolicy, nil
}

func crossNamespaceRules(partitionInfo PartitionInfo) (string, error) {
	crossNamespaceRulesTpl := `{{- if .EnablePartitions }}
partition "{{ .PartitionName }}" {
{{- end }}
  namespace_prefix "" {
    service_prefix "" {
      policy = "read"
    }
    node_prefix "" {
      policy = "read"
    }
  }
{{- if .EnablePartitions }}
}
{{- end }}`

	compiled, err := template.New("root").Parse(strings.TrimSpace(crossNamespaceRulesTpl))
	if err != nil {
		return "", err
	}

	// Render the template
	var buf bytes.Buffer
	err = compiled.Execute(&buf, partitionInfo)
	if err != nil {
		// Discard possible partial results on error return
		return "", err
	}

	return buf.String(), nil
}
