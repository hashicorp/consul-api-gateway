package reconciler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gw "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestSetAdmittedStatus(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		status         gw.RouteStatus
		parentStatuses []gw.RouteParentStatus
		expected       gw.RouteStatus
	}{{
		name:           "empty",
		status:         gw.RouteStatus{},
		parentStatuses: []gw.RouteParentStatus{},
		expected:       gw.RouteStatus{},
	}, {
		name: "basic",
		status: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
		parentStatuses: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
			Controller: "expected",
			Conditions: []meta.Condition{{
				Type:    string(gw.ConditionRouteAdmitted),
				Status:  meta.ConditionTrue,
				Message: "true",
			}},
		}},
		expected: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionTrue,
					Message: "true",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
	}, {
		name:   "add",
		status: gw.RouteStatus{},
		parentStatuses: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
			Controller: "expected",
			Conditions: []meta.Condition{{
				Type:    string(gw.ConditionRouteAdmitted),
				Status:  meta.ConditionTrue,
				Message: "true",
			}},
		}},
		expected: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionTrue,
					Message: "true",
				}},
			}},
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			actual := setAdmittedStatus(test.status, test.parentStatuses...)
			require.ElementsMatch(t, test.expected.Parents, actual.Parents)
		})
	}
}

func TestSetResolvedRefStatus(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		status         gw.RouteStatus
		parentStatuses []gw.RouteParentStatus
		expected       gw.RouteStatus
	}{{
		name:           "empty",
		status:         gw.RouteStatus{},
		parentStatuses: []gw.RouteParentStatus{},
		expected:       gw.RouteStatus{},
	}, {
		name: "basic",
		status: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
		parentStatuses: []gw.RouteParentStatus{{
			ParentRef: gw.ParentRef{
				Name: "expected",
			},
			Controller: "expected",
			Conditions: []meta.Condition{{
				Type:    string(gw.ConditionRouteResolvedRefs),
				Status:  meta.ConditionTrue,
				Message: "true",
			}},
		}},
		expected: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionTrue,
					Message: "true",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			actual := setResolvedRefsStatus(test.status, test.parentStatuses...)
			require.ElementsMatch(t, test.expected.Parents, actual.Parents)
		})
	}
}

func TestClearParentStatus(t *testing.T) {
	t.Parallel()

	expected := gw.Namespace("expected")

	for _, test := range []struct {
		name           string
		status         gw.RouteStatus
		namespace      string
		controllerName string
		gatewayName    types.NamespacedName
		expected       gw.RouteStatus
	}{{
		name: "empty",
	}, {
		name:           "basic",
		namespace:      "test",
		controllerName: "expected",
		gatewayName: types.NamespacedName{
			Name:      "expected",
			Namespace: "expected",
		},
		status: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name:      "expected",
					Namespace: &expected,
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
		expected: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
	}, {
		name:           "local-namespace",
		namespace:      "expected",
		controllerName: "expected",
		gatewayName: types.NamespacedName{
			Name:      "expected",
			Namespace: "expected",
		},
		status: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "expected",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}, {
					Type:    string(gw.ConditionRouteResolvedRefs),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
		expected: gw.RouteStatus{
			Parents: []gw.RouteParentStatus{{
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}, {
				ParentRef: gw.ParentRef{
					Name: "expected",
				},
				Controller: "other",
				Conditions: []meta.Condition{{
					Type:    string(gw.ConditionRouteAdmitted),
					Status:  meta.ConditionFalse,
					Message: "false",
				}},
			}},
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			actual := clearParentStatus(test.controllerName, test.namespace, test.status, test.gatewayName)
			require.ElementsMatch(t, test.expected.Parents, actual.Parents)
		})
	}
}

func TestUpdateCondition(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		current      meta.Condition
		updated      meta.Condition
		expectUpdate bool
	}{{
		name:         "generation",
		expectUpdate: true,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 1,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
	}, {
		name:         "message",
		expectUpdate: true,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "b",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
	}, {
		name:         "reason",
		expectUpdate: true,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "b",
			Status:             meta.ConditionTrue,
		},
	}, {
		name:         "status",
		expectUpdate: true,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionFalse,
		},
	}, {
		name:         "generation-keep",
		expectUpdate: false,
		current: meta.Condition{
			ObservedGeneration: 1,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "b",
			Status:             meta.ConditionFalse,
		},
	}, {
		name:         "generation-match",
		expectUpdate: true,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "b",
			Status:             meta.ConditionFalse,
		},
	}, {
		name:         "keep",
		expectUpdate: false,
		current: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
			LastTransitionTime: meta.Now(),
		},
		updated: meta.Condition{
			ObservedGeneration: 0,
			Message:            "a",
			Reason:             "a",
			Status:             meta.ConditionTrue,
			LastTransitionTime: meta.NewTime(meta.Now().Add(100 * time.Minute)),
		},
	}} {
		t.Run(test.name, func(t *testing.T) {
			condition := updateCondition(test.current, test.updated)
			if test.expectUpdate {
				require.Equal(t, test.updated, condition)
			} else {
				require.Equal(t, test.current, condition)
			}
		})
	}
}
