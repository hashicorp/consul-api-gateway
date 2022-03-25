package reconciler

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/service/mocks"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteValidate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	resolver := mocks.NewMockBackendResolver(ctrl)

	validator := NewRouteValidator(resolver)
	factory := NewFactory(FactoryConfig{
		Logger: hclog.NewNullLogger(),
	})

	route := factory.NewRoute(&gw.HTTPRoute{})
	state, err := validator.Validate(context.Background(), route)
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

	route = factory.NewRoute(&gw.HTTPRoute{
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
	state, err = validator.Validate(context.Background(), route)
	require.NoError(t, err)
	require.True(t, state.ResolutionErrors.Empty())

	expected := errors.New("expected")
	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, expected)
	_, err = validator.Validate(context.Background(), route)
	require.Equal(t, expected, err)

	resolver.EXPECT().Resolve(gomock.Any(), gomock.Any(), reference).Return(nil, service.NewK8sResolutionError("error"))
	state, err = validator.Validate(context.Background(), route)
	require.NoError(t, err)
	require.False(t, state.ResolutionErrors.Empty())
}
