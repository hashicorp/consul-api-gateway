package validator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	clientMocks "github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/reconciler/state"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
)

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)
	routeState, err := validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{})
	require.NoError(t, err)
	require.True(t, routeState.ResolutionErrors.Empty())

	reference := gwv1alpha2.BackendObjectReference{
		Name: "expected",
	}
	resolved := &service.ResolvedReference{
		Type:      service.ConsulServiceReference,
		Reference: &service.BackendReference{},
	}
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(resolved, nil)

	routeState, err = validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.NoError(t, err)
	require.True(t, routeState.ResolutionErrors.Empty())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, expected)
	routeState, err = validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.Equal(t, expected, err)

	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))

	routeState = state.NewRouteState()
	routeState, err = validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: reference,
					},
				}},
			}},
		},
	})
	require.NoError(t, err)
	require.False(t, routeState.ResolutionErrors.Empty())
}

func TestRouteValidateDontAllowCrossNamespace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)
	namespace := gwv1alpha2.Namespace("test")

	client.EXPECT().
		GetReferenceGrantsInNamespace(gomock.Any(), gomock.Any()).
		Return([]gwv1alpha2.ReferenceGrant{
			{
				Spec: gwv1alpha2.ReferenceGrantSpec{
					From: []gwv1alpha2.ReferenceGrantFrom{},
					To:   []gwv1alpha2.ReferenceGrantTo{},
				},
			},
		}, nil)

	ref := gwv1alpha2.BackendRef{
		BackendObjectReference: gwv1alpha2.BackendObjectReference{
			Name:      "expected",
			Namespace: &namespace,
		},
	}

	routeState, err := validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: ref,
				}},
			}},
		},
	})
	require.NoError(t, err)

	require.Contains(t, routeState.ResolutionErrors.String(), "Cross-namespace routing not allowed without matching ReferencePolicy")
}

func TestRouteValidateAllowCrossNamespaceWithReferenceGrant(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)
	client := clientMocks.NewMockClient(ctrl)

	validator := NewRouteValidator(resolver, client)

	//set up backend ref with a different namespace
	backendGroup := gwv1alpha2.Group("")
	backendKind := gwv1alpha2.Kind("Service")
	backendNamespace := gwv1alpha2.Namespace("namespace2")
	backendName := gwv1alpha2.ObjectName("backend2")

	referencePolicy := gwv1alpha2.ReferenceGrant{
		TypeMeta:   meta.TypeMeta{},
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace2"},
		Spec: gwv1alpha2.ReferenceGrantSpec{
			From: []gwv1alpha2.ReferenceGrantFrom{{
				Group:     "gateway.networking.k8s.io",
				Kind:      "HTTPRoute",
				Namespace: "namespace1",
			}},
			To: []gwv1alpha2.ReferenceGrantTo{{
				Group: "",
				Kind:  "Service",
				Name:  &backendName,
			}},
		},
	}

	client.EXPECT().
		GetReferenceGrantsInNamespace(gomock.Any(), gomock.Any()).
		Return([]gwv1alpha2.ReferenceGrant{referencePolicy}, nil)

	resolver.EXPECT().
		Resolve(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&service.ResolvedReference{Type: service.ConsulServiceReference, Reference: &service.BackendReference{}}, nil)

	routeState, err := validator.Validate(context.Background(), &gwv1alpha2.HTTPRoute{
		ObjectMeta: meta.ObjectMeta{Namespace: "namespace1"},
		TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
		Spec: gwv1alpha2.HTTPRouteSpec{
			Rules: []gwv1alpha2.HTTPRouteRule{{
				BackendRefs: []gwv1alpha2.HTTPBackendRef{{
					BackendRef: gwv1alpha2.BackendRef{
						BackendObjectReference: gwv1alpha2.BackendObjectReference{
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
	require.Empty(t, routeState.ResolutionErrors.String())
}

func TestRouteAllowedForBackendRef(t *testing.T) {
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
	backend1, backend2, backend3 := "backend1", "backend2", "backend3"

	for _, tc := range []testCase{
		{name: "unspecified-backend-namespace-allowed", fromNS: ns1, toNS: nil, toName: backend1, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "same-namespace-no-name-allowed", fromNS: ns1, toNS: &ns1, toName: backend1, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "same-namespace-with-name-allowed", fromNS: ns1, toNS: &ns1, toName: backend1, grantFromNS: ns1, grantToName: &backend1, allowed: true},
		{name: "different-namespace-no-name-allowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: nil, allowed: true},
		{name: "different-namespace-with-name-allowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: &backend2, allowed: true},
		{name: "mismatched-grant-from-namespace-disallowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns3, grantToName: &backend2, allowed: false},
		{name: "mismatched-grant-to-name-disallowed", fromNS: ns1, toNS: &ns2, toName: backend2, grantFromNS: ns1, grantToName: &backend3, allowed: false},
	} {
		// Test each case for both HTTPRoute + TCPRoute which should function identically
		for _, routeType := range []string{"HTTPRoute", "TCPRoute"} {
			t.Run(tc.name+"-for-"+routeType, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := clientMocks.NewMockClient(ctrl)

				group := gwv1alpha2.Group("")

				backendRef := gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group: &group,
						Name:  gwv1alpha2.ObjectName(tc.toName),
					},
				}

				if tc.toNS != nil {
					ns := gwv1alpha2.Namespace(*tc.toNS)
					backendRef.BackendObjectReference.Namespace = &ns
				}

				if tc.toKind != nil {
					k := gwv1alpha2.Kind(*tc.toKind)
					backendRef.Kind = &k
				}

				var route Route
				switch routeType {
				case "HTTPRoute":
					route = &gwv1alpha2.HTTPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "HTTPRoute"},
						Spec: gwv1alpha2.HTTPRouteSpec{
							Rules: []gwv1alpha2.HTTPRouteRule{{
								BackendRefs: []gwv1alpha2.HTTPBackendRef{{BackendRef: backendRef}},
							}},
						},
					}
				case "TCPRoute":
					route = &gwv1alpha2.TCPRoute{
						ObjectMeta: meta.ObjectMeta{Namespace: tc.fromNS},
						TypeMeta:   meta.TypeMeta{APIVersion: "gateway.networking.k8s.io/v1alpha2", Kind: "TCPRoute"},
						Spec: gwv1alpha2.TCPRouteSpec{
							Rules: []gwv1alpha2.TCPRouteRule{{
								BackendRefs: []gwv1alpha2.BackendRef{backendRef},
							}},
						},
					}
				default:
					require.Fail(t, fmt.Sprintf("unhandled route type %q", routeType))
				}

				var toName *gwv1alpha2.ObjectName
				if tc.grantToName != nil {
					on := gwv1alpha2.ObjectName(*tc.grantToName)
					toName = &on
				}

				if tc.toNS != nil && tc.fromNS != *tc.toNS {
					referenceGrant := gwv1alpha2.ReferenceGrant{
						TypeMeta:   meta.TypeMeta{},
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{
								Group:     "gateway.networking.k8s.io",
								Kind:      gwv1alpha2.Kind(routeType),
								Namespace: gwv1alpha2.Namespace(tc.grantFromNS),
							}},
							To: []gwv1alpha2.ReferenceGrantTo{{
								Group: "",
								Kind:  "Service",
								Name:  toName,
							}},
						},
					}

					throwawayGrant := gwv1alpha2.ReferenceGrant{
						ObjectMeta: meta.ObjectMeta{Namespace: *tc.toNS},
						Spec: gwv1alpha2.ReferenceGrantSpec{
							From: []gwv1alpha2.ReferenceGrantFrom{{
								Group:     "Kool & The Gang",
								Kind:      "Jungle Boogie",
								Namespace: "Wild And Peaceful",
							}},
							To: []gwv1alpha2.ReferenceGrantTo{{
								Group: "does not exist",
								Kind:  "does not exist",
								Name:  nil,
							}},
						},
					}

					client.EXPECT().
						GetReferenceGrantsInNamespace(gomock.Any(), *tc.toNS).
						Return([]gwv1alpha2.ReferenceGrant{throwawayGrant, referenceGrant}, nil)
				}

				allowed, err := routeAllowedForBackendRef(context.Background(), route, backendRef, client)
				require.NoError(t, err)
				assert.Equal(t, tc.allowed, allowed)
			})
		}
	}
}
