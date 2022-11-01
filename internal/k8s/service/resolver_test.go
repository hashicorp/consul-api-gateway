package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	mocks2 "github.com/hashicorp/consul-api-gateway/internal/consul/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/gatewayclient/mocks"
	"github.com/hashicorp/consul-api-gateway/internal/k8s/utils"
	"github.com/hashicorp/consul-api-gateway/pkg/apis/v1alpha1"
)

var sameNamespaceMapper = func(ns string) string { return ns }

func getLengthAtErrorType(r *ResolutionErrors, errType ServiceResolutionErrorType) int {
	return len(r.errors[errType])
}

func TestResolutionErrors_Add(t *testing.T) {
	r := NewResolutionErrors()

	r.Add(NewConsulResolutionError("consul error"))
	assert.Equal(t, getLengthAtErrorType(r, ConsulServiceResolutionErrorType), 1)

	r.Add(NewK8sResolutionError("k8s error"))
	assert.Equal(t, getLengthAtErrorType(r, K8sServiceResolutionErrorType), 1)

	r.Add(NewResolutionError("generic error"))
	assert.Equal(t, getLengthAtErrorType(r, GenericResolutionErrorType), 1)

	r.Add(NewRefNotPermittedError("refnotpermitted error"))
	assert.Equal(t, getLengthAtErrorType(r, RefNotPermittedErrorType), 1)

	r.Add(NewInvalidKindError("invalidkind error"))
	assert.Equal(t, getLengthAtErrorType(r, InvalidKindErrorType), 1)

	r.Add(NewBackendNotFoundError("backendnotfound error"))
	assert.Equal(t, getLengthAtErrorType(r, BackendNotFoundErrorType), 1)
}

func TestResolutionErrors_Flatten(t *testing.T) {
	type fields struct {
		errors map[ServiceResolutionErrorType][]ResolutionError
	}
	tests := []struct {
		name    string
		fields  fields
		want    ServiceResolutionErrorType
		wantErr bool
	}{
		{
			name:    "no errors",
			want:    NoResolutionErrorType,
			wantErr: false,
		},
		{
			name:    "refNotPermittedErrors error",
			want:    RefNotPermittedErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					RefNotPermittedErrorType: {
						NewRefNotPermittedError("expected"),
					},
				},
			},
		},
		{
			name:    "invalidkind error",
			want:    InvalidKindErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					InvalidKindErrorType: {
						NewInvalidKindError("expected"),
					},
				},
			},
		},
		{
			name:    "backendnotfound error",
			want:    BackendNotFoundErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					BackendNotFoundErrorType: {
						NewBackendNotFoundError("expected"),
					},
				},
			},
		},
		{
			name:    "generic error",
			want:    GenericResolutionErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					GenericResolutionErrorType: {
						NewResolutionError("expected"),
					},
				},
			},
		},

		{
			name:    "k8s error",
			want:    K8sServiceResolutionErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					K8sServiceResolutionErrorType: {
						NewK8sResolutionError("expected"),
					},
				},
			},
		},

		{
			name:    "consul error",
			want:    ConsulServiceResolutionErrorType,
			wantErr: true,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					ConsulServiceResolutionErrorType: {
						NewConsulResolutionError("expected"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ResolutionErrors{
				errors: tt.fields.errors,
			}
			got, err := r.Flatten()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, got, tt.want)
		})
	}
}

func TestResolutionErrors_Empty(t *testing.T) {
	type fields struct {
		errors map[ServiceResolutionErrorType][]ResolutionError
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "no errors",
			want: true,
		},
		{
			name: "refNotPermittedErrors error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					RefNotPermittedErrorType: {
						NewRefNotPermittedError("expected"),
					},
				},
			},
		},
		{
			name: "invalidkind error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					InvalidKindErrorType: {
						NewInvalidKindError("expected"),
					},
				},
			},
		},
		{
			name: "backendnotfound error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					BackendNotFoundErrorType: {
						NewBackendNotFoundError("expected"),
					},
				},
			},
		},
		{
			name: "generic error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					GenericResolutionErrorType: {
						NewResolutionError("expected"),
					},
				},
			},
		},

		{
			name: "k8s error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					K8sServiceResolutionErrorType: {
						NewK8sResolutionError("expected"),
					},
				},
			},
		},

		{
			name: "consul error",
			want: false,
			fields: fields{
				errors: map[ServiceResolutionErrorType][]ResolutionError{
					ConsulServiceResolutionErrorType: {
						NewConsulResolutionError("expected"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ResolutionErrors{
				errors: tt.fields.errors,
			}
			got := r.Empty()
			require.Equal(t, got, tt.want)
		})
	}
}

func TestBackendResolver_consulServiceForMeshService_peer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	meshService := &v1alpha1.MeshService{
		ObjectMeta: meta.ObjectMeta{
			Namespace: t.Name(),
			Name:      "mesh_service",
		},
		Spec: v1alpha1.MeshServiceSpec{
			Name: "imported_service",
			Peer: pointer.String("exporting_peer"),
		},
	}

	peering := &api.Peering{
		StreamStatus: api.PeeringStreamStatus{
			ImportedServices: []string{"imported_service"},
		},
	}

	gwClient := mocks.NewMockClient(ctrl)
	gwClient.EXPECT().GetMeshService(gomock.Any(), utils.NamespacedName(meshService)).Return(meshService, nil)

	peerings := mocks2.NewMockPeerings(ctrl)
	peerings.EXPECT().Read(gomock.Any(), "exporting_peer", &api.QueryOptions{Namespace: t.Name()}).Return(peering, nil, nil)

	resolver := &backendResolver{
		client:   gwClient,
		logger:   hclog.NewNullLogger(),
		mapper:   sameNamespaceMapper,
		peerings: peerings,
	}

	ref, err := resolver.consulServiceForMeshService(context.Background(), utils.NamespacedName(meshService))
	require.NoError(t, err)
	require.NotNil(t, ref)
	require.NotNil(t, ref.Consul)
	assert.Equal(t, meshService.Namespace, ref.Consul.Namespace)
	assert.Equal(t, "imported_service", ref.Consul.Name)
}
