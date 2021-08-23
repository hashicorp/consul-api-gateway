package object

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeObj is implemented by top level Kubernetes objects such a gateway.Gateway
type KubeObj interface {
	client.Object
	schema.ObjectKind
}

// Object is a wrapper for a top level Kubernetes object that allows for tracking mutations to the objects status
// as well as providing a getter for the spec in a generic way.
type Object struct {
	KubeObj
	spec   reflect.Value
	Status Status
}

// Value returns the wrapped Kubernetes object.
func (o *Object) Value() KubeObj {
	return o.KubeObj
}

// DeepCopyObject overrides the embedded KubeObj implementation to return a deep copy of the Object
func (o *Object) DeepCopyObject() runtime.Object {
	if obj, ok := o.KubeObj.DeepCopyObject().(KubeObj); ok {
		return New(obj)
	}
	return nil
}

// GetSpec returns a copy of the object's Spec
func (o *Object) GetSpec() interface{} {
	var spec reflect.Value
	reflect.Copy(spec, o.spec.Elem())
	return spec.Interface()
}

// Status is a wrapper for a Kubernetes' object status to provide tracking of mutations.
type Status interface {
	// Mutate expects a callback which passes the current status object as a pointer and expects it to return an updated status.
	// If the status was changed it will be marked as dirty, otherwise it will not change the dirty flag.
	// Mutate will never change dirty from true to false.
	Mutate(func(status interface{}) interface{})

	// IsDirty returns a bool indicating if the Status has been mutated
	IsDirty() bool
	// SetDirty will set the dirty flag to the given bool argument
	SetDirty(bool)
}

// New expects a KubeObj and returns a wrapped Object
func New(obj KubeObj) *Object {
	o := &Object{
		KubeObj: obj,
	}

	// find the Spec and Status field values
	val := reflect.ValueOf(obj).Elem()
	var specVal, statusVal reflect.Value
	for i := 0; i < val.NumField(); i++ {
		switch val.Type().Field(i).Name {
		case "Status":
			statusVal = val.Field(i)
		case "Spec":
			specVal = val.Field(i)
		}
	}

	// spec and status values are referenced as a pointers to the wrapped obj fields
	o.spec = specVal.Addr()
	o.Status = &status{
		status: statusVal.Addr().Interface(),
	}

	return o
}

// status implements the Status interface
type status struct {
	status interface{}
	dirty  bool
}

func (s *status) IsDirty() bool {
	return s.dirty
}

func (s *status) SetDirty(d bool) {
	s.dirty = d
}

// deepCopy makes use of the generated json struct tags of Kubernetes status objects to copy the status object
func (s *status) deepCopy() interface{} {
	raw, err := json.Marshal(s.status)
	if err != nil {
		return nil
	}

	status := reflect.New(reflect.TypeOf(s.status).Elem()).Interface()
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil
	}
	return status
}

func (s *status) Mutate(f func(status interface{}) interface{}) {
	if s.status == nil {
		return
	}
	old := s.deepCopy()
	s.status = f(s.status)
	if !reflect.DeepEqual(old, s.status) {
		s.dirty = true
	}
}
