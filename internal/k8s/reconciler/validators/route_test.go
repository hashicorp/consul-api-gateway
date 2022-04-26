package validators

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)
	state, err := validator.Validate(context.Background(), &gw.HTTPRoute{})
	require.NoError(t, err)
	require.True(t, state.ResolutionErrors.Empty())

	reference := gw.BackendObjectReference{
		Name: "expected",
	}
	resolved := &service.ResolvedReference{
		Type:      service.ConsulServiceReference,
		Reference: &service.BackendReference{},
	}
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(resolved, nil)

	state, err = validator.Validate(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.NoError(t, err)
	require.True(t, state.ResolutionErrors.Empty())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, expected)
	_, err = validator.Validate(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.Equal(t, expected, err)

	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))
	state, err = validator.Validate(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.NoError(t, err)
	require.False(t, state.ResolutionErrors.Empty())
}

func TestRouteValidateDontAllowCrossNamespace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)
	namespace := gw.Namespace("test")

	client.EXPECT().
		GetReferencePoliciesInNamespace(gomock.Any(), gomock.Any()).
		Return([]gw.ReferencePolicy{
			{
				Spec: gw.ReferencePolicySpec{
					From: []gw.ReferencePolicyFrom{},
					To:   []gw.ReferencePolicyTo{},
				},
			},
		}, nil)

	ref := gw.BackendRef{
		BackendObjectReference: gw.BackendObjectReference{
			Name:      "expected",
			Namespace: &namespace,
		},
	}

	state, err := validator.Validate(context.Background(), &gw.HTTPRoute{
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: ref,
				}},
			}},
		},
	})
	require.NoError(t, err)

	require.Contains(t, state.ResolutionErrors.String(), "Cross-namespace routing not allowed without matching ReferencePolicy")
}

func TestRouteValidateAllowCrossNamespaceWithReferencePolicy(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)

	//set up backend ref with a different namespace
	backendGroup := gw.Group("")
	backendKind := gw.Kind("Service")
	backendNamespace := gw.Namespace("namespace2")
	backendName := gw.ObjectName("backend2")

	referencePolicy := gw.ReferencePolicy{
		TypeMeta:   meta.TypeMeta{},
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace2"},
		Spec: gw.ReferencePolicySpec{
			From: []gw.ReferencePolicyFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gw.ReferencePolicyTo{{
				Group: "",
				Kind:  "Service",
				Name:  &backendName,
			}},
		},
	}

	client.EXPECT().
		GetReferencePoliciesInNamespace(gomock.Any(), gomock.Any()).
		Return([]gw.ReferencePolicy{referencePolicy}, nil)

	resolver.EXPECT().
		Resolve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&service.ResolvedReference{Type: service.ConsulServiceReference, Reference: &service.BackendReference{}}, nil)

	state, err := validator.Validate(context.Background(), &gw.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace1"},
		TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
		Spec: gw.HTTPRouteSpec{
			Rules: []gw.HTTPRouteRule{{
				BackendRefs: []gw.HTTPBackendRef{{
					BackendRef: gw.BackendRef{
						BackendObjectReference: gw.BackendObjectReference{
							Group:     &backendGroup,
							Kind:      &backendKind,
							Name:      backendName,
							Namespace: &backendNamespace,
						},
					},
				}},
			}},
		},
	})

	require.NoError(t, err)
	require.Empty(t, state.ResolutionErrors.String())
}

func TestRouteAllowedForBackendRef(t *testing.T) {
	type testCase struct {
		name         string
		routeNS      string
		backendNS    *string
		backendKind  *string
		backendName  string
		policyFromNS string
		policyToName *string
		allowed      bool
	}

	ns1, ns2, ns3 := "namespace1", "namespace2", "namespace3"
	backend1, backend2, backend3 := "backend1", "backend2", "backend3"

	for _, tc := range []testCase{
		{name: "unspecified-backend-namespace-allowed", routeNS: ns1, backendNS: nil, backendName: backend1, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", routeNS: ns1, backendNS: &ns1, backendName: backend1, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", routeNS: ns1, backendNS: &ns1, backendName: backend1, policyFromNS: ns1, policyToName: &backend1, allowed: true},
		{name: "different-namespace-no-name-allowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: &backend2, allowed: true},
		{name: "mismatched-policy-from-namespace-disallowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns3, policyToName: &backend2, allowed: false},
		{name: "mismatched-policy-to-name-disallowed", routeNS: ns1, backendNS: &ns2, backendName: backend2, policyFromNS: ns1, policyToName: &backend3, allowed: false},
	} {
		// Test each case for both HTTPRoute + TCPRoute which should function identically
		for _, routeType := range []string{"HTTPRoute", "TCPRoute"} {
			t.Run(tc.name+"-for-"+routeType, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := clientMocks.NewMockClient(ctrl)

				group := gw.Group("")

				backendRef := gw.BackendRef{
					BackendObjectReference: gw.BackendObjectReference{
						Group: &group,
						Name:  gw.ObjectName(tc.backendName),
					},
				}

				if tc.backendNS != nil {
					ns := gw.Namespace(*tc.backendNS)
					backendRef.BackendObjectReference.Namespace = &ns
				}

				if tc.backendKind != nil {
					k := gw.Kind(*tc.backendKind)
					backendRef.Kind = &k
				}

				var route Route
				switch routeType {
				case "HTTPRoute":
					route = &gw.HTTPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.routeNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
						Spec: gw.HTTPRouteSpec{
							Rules: []gw.HTTPRouteRule{{
								BackendRefs: []gw.HTTPBackendRef{{BackendRef: backendRef}},
							}},
						},
					}
				case "TCPRoute":
					route = &gw.TCPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.routeNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "TCPRoute"},
						Spec: gw.TCPRouteSpec{
							Rules: []gw.TCPRouteRule{{
								BackendRefs: []gw.BackendRef{backendRef},
							}},
						},
					}
				default:
					require.Fail(t, fmt.Sprintf("unhandled route type %q", routeType))
				}

				var toName *gw.ObjectName
				if tc.policyToName != nil {
					on := gw.ObjectName(*tc.policyToName)
					toName = &on
				}

				if tc.backendNS != nil && tc.routeNS != *tc.backendNS {
					referencePolicy := gw.ReferencePolicy{
						TypeMeta:   meta.TypeMeta{},
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.backendNS},
						Spec: gw.ReferencePolicySpec{
							From: []gw.ReferencePolicyFrom{{
								Group:     "gateway.networking.k8s.io",
								Kind:      gw.Kind(routeType),
								Namespace: gw.Namespace(tc.policyFromNS),
							}},
							To: []gw.ReferencePolicyTo{{
								Group: "",
								Kind:  "Service",
								Name:  toName,
							}},
						},
					}

					throwawayPolicy := gw.ReferencePolicy{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.backendNS},
						Spec: gw.ReferencePolicySpec{
							From: []gw.ReferencePolicyFrom{{
								Group:     "Kool & The Gang",
								Kind:      "Jungle Boogie",
								Namespace: "Wild And Peaceful",
							}},
							To: []gw.ReferencePolicyTo{{
								Group: "does not exist",
								Kind:  "does not exist",
								Name:  nil,
							}},
						},
					}

					client.EXPECT().
						GetReferencePoliciesInNamespace(gomock.Any(), *tc.backendNS).
						Return([]gw.ReferencePolicy{throwawayPolicy, referencePolicy}, nil)
				}

				allowed, err := routeAllowedForBackendRef(context.Background(), route, backendRef, client)
				require.NoError(t, err)
				assert.Equal(t, tc.allowed, allowed)
			})
		}
	}
}
