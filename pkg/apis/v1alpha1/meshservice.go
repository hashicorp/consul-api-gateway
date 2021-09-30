package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	MeshServiceKind = "MeshService"
)

// +genclient
// +kubebuilder:object:root=true

// MeshService holds a reference to an externally managed Consul Service Mesh service.
type MeshService struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of MeshService.
	Spec MeshServiceSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen=true

// MeshServiceSpec specifies the 'spec' of the MeshService CRD.
type MeshServiceSpec struct {
	// Name holds the service name for a consul service.
	Name string `json:"name,omitempty"`
	// Namespace holds the namespace information about a consul service
	Namespace string `json:"namespace,omitempty"`
	// Datacenter holds the datacenter information about a consul service
	Datacenter string `json:"datacenter,omitempty"`
}

// +kubebuilder:object:root=true

// MeshServiceList is a list of MeshService resources.
type MeshServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MeshService `json:"items"`
}
