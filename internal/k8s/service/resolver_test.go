package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolutionErrors_Add(t *testing.T) {

	r := &ResolutionErrors{}
	r.Add(NewConsulResolutionError("consul error"))
	require.Equal(t, len(r.consulErrors), 1)

	r.Add(NewK8sResolutionError("k8s error"))
	require.Equal(t, len(r.k8sErrors), 1)

	r.Add(NewResolutionError("generic error"))
	require.Equal(t, len(r.genericErrors), 1)

	r.Add(NewRefNotPermittedError("refnotpermitted error"))
	require.Equal(t, len(r.refNotPermittedErrors), 1)

}

func TestResolutionErrors_Flatten(t *testing.T) {
	type fields struct {
		k8sErrors             []ResolutionError
		consulErrors          []ResolutionError
		genericErrors         []ResolutionError
		refNotPermittedErrors []ResolutionError
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
				refNotPermittedErrors: []ResolutionError{NewRefNotPermittedError("expected")},
			},
		},
		{
			name:    "generic error",
			want:    GenericResolutionErrorType,
			wantErr: true,
			fields: fields{
				genericErrors: []ResolutionError{NewResolutionError("expected")},
			},
		},
		{
			name:    "k8s error",
			want:    K8sServiceResolutionErrorType,
			wantErr: true,
			fields: fields{
				k8sErrors: []ResolutionError{NewK8sResolutionError("expected")},
			},
		},
		{
			name:    "consul error",
			want:    ConsulServiceResolutionErrorType,
			wantErr: true,
			fields: fields{
				consulErrors: []ResolutionError{NewConsulResolutionError("expected")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ResolutionErrors{
				k8sErrors:             tt.fields.k8sErrors,
				consulErrors:          tt.fields.consulErrors,
				genericErrors:         tt.fields.genericErrors,
				refNotPermittedErrors: tt.fields.refNotPermittedErrors,
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
		k8sErrors             []ResolutionError
		consulErrors          []ResolutionError
		genericErrors         []ResolutionError
		refNotPermittedErrors []ResolutionError
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "is empty",
			want: true,
		},
		{
			name: "refNotPermittedErrors error",
			want: false,
			fields: fields{
				refNotPermittedErrors: []ResolutionError{NewRefNotPermittedError("expected")},
			},
		},
		{
			name: "generic error",
			want: false,
			fields: fields{
				genericErrors: []ResolutionError{NewResolutionError("expected")},
			},
		},
		{
			name: "consul error",
			want: false,
			fields: fields{
				consulErrors: []ResolutionError{NewConsulResolutionError("expected")},
			},
		},
		{
			name: "k8s error",
			want: false,
			fields: fields{
				k8sErrors: []ResolutionError{NewK8sResolutionError("expected")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ResolutionErrors{
				k8sErrors:             tt.fields.k8sErrors,
				consulErrors:          tt.fields.consulErrors,
				genericErrors:         tt.fields.genericErrors,
				refNotPermittedErrors: tt.fields.refNotPermittedErrors,
			}
			got := r.Empty()
			require.Equal(t, got, tt.want)
		})
	}
}
