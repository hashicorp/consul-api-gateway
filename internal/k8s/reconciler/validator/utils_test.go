package validator

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
)

func TestGatewayAllowedForSecretRef(t *testing.T) {
	type testCase struct {
		name        string
		fromNS      string
		toNS        *string
		toKind      *string
		toName      string
		grantFromNS string
		grantToName *string
		allowed     bool
	}

	ns1, ns2, ns3 := "namespace1", "namespace2", "namespace3"
	secret1, secret2, secret3 := "secret1", "secret2", "secret3"

	for _, tc := range []testCase{
		{name: "unspecified-secret-namespace-allowed", fromNS: ns1, toNS: nil, toName: secret1, grantToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", fromNS: ns1, toNS: &ns1, toName: secret1, grantToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", fromNS: ns1, toNS: &ns1, toName: secret1, grantToName: &secret1, allowed: true},
		{name: "different-namespace-no-name-allowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: &secret2, allowed: true},
		{name: "mismatched-grant-from-namespace-disallowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns3, grantToName: &secret2, allowed: false},
		{name: "mismatched-grant-to-name-disallowed", fromNS: ns1, toNS: &ns2, toName: secret2, grantFromNS: ns1, grantToName: &secret3, allowed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := mocks.NewMockClient(ctrl)

			group := gwv1beta1.Group("")

			secretRef := gwv1beta1.SecretObjectReference{
				Group: &group,
				Name:  gwv1beta1.ObjectName(tc.toName),
			}

			if tc.toNS != nil {
				ns := gwv1beta1.Namespace(*tc.toNS)
				secretRef.Namespace = &ns
			}

			if tc.toKind != nil {
				k := gwv1beta1.Kind(*tc.toKind)
				secretRef.Kind = &k
			}

			gateway := &gwv1beta1.Gateway{
				TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "Gateway"},
				ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{{
						TLS: &gwv1beta1.GatewayTLSConfig{
							CertificateRefs: []gwv1beta1.SecretObjectReference{{
								Group: &group,
								Name:  gwv1beta1.ObjectName(tc.toName),
							}},
						},
					}},
				},
			}

			var toName *gwv1alpha2.ObjectName
			if tc.grantToName != nil {
				on := gwv1alpha2.ObjectName(*tc.grantToName)
				toName = &on
			}

			if tc.toNS != nil && tc.fromNS != *tc.toNS {
				otherName := gwv1alpha2.ObjectName("blah")

				refGrants := []gwv1alpha2.ReferenceGrant{
					// Create a ReferenceGrant that does not match at all (kind, etc.)
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "Kool & The Gang", Kind: "Jungle Boogie", Namespace: "Wild And Peaceful"}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "does not exist", Kind: "does not exist", Name: nil}},
						},
					},
					// Create a ReferenceGrant that matches completely except for To.Name
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "gateway.networking.k8s.io", Kind: gwv1alpha2.Kind("Gateway"), Namespace: gwv1alpha2.Namespace(tc.grantFromNS)}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "", Kind: "Secret", Name: &otherName}},
						},
					},
					// Create a ReferenceGrant that matches completely
					{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{Group: "gateway.networking.k8s.io", Kind: gwv1alpha2.Kind("Gateway"), Namespace: gwv1alpha2.Namespace(tc.grantFromNS)}},
							To:   []gwv1alpha2.ReferenceGrantTo{{Group: "", Kind: "Secret", Name: toName}},
						},
					},
				}

				client.EXPECT().
					GetReferenceGrantsInNamespace(gomock.Any(), *tc.toNS).
					Return(refGrants, nil)
			}

			allowed, err := gatewayAllowedForSecretRef(context.Background(), gateway, secretRef, client)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)
		})
	}
}
