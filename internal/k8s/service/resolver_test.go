package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
