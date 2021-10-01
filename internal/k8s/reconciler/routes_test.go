package reconciler

// func TestRouteSetStatus(t *testing.T) {
// 	t.Parallel()

// 	status := gw.RouteStatus{
// 		Parents: []gw.RouteParentStatus{{
// 			ParentRef: gw.ParentRef{
// 				Name: "expected",
// 			},
// 			Controller: "other",
// 			Conditions: []meta.Condition{{
// 				Type:    string(gw.ConditionRouteAdmitted),
// 				Status:  meta.ConditionFalse,
// 				Message: "false",
// 			}},
// 		}, {
// 			ParentRef: gw.ParentRef{
// 				Name: "expected",
// 			},
// 			Controller: "expected",
// 			Conditions: []meta.Condition{{
// 				Type:    string(gw.ConditionRouteAdmitted),
// 				Status:  meta.ConditionFalse,
// 				Message: "false",
// 			}, {
// 				Type:    string(gw.ConditionRouteResolvedRefs),
// 				Status:  meta.ConditionFalse,
// 				Message: "false",
// 			}},
// 		}, {
// 			ParentRef: gw.ParentRef{
// 				Name: "expected",
// 			},
// 			Controller: "other",
// 			Conditions: []meta.Condition{{
// 				Type:    string(gw.ConditionRouteAdmitted),
// 				Status:  meta.ConditionFalse,
// 				Message: "false",
// 			}},
// 		}},
// 	}

// 	lastIndex := len(status.Parents) - 1

// 	for _, test := range []struct {
// 		name     string
// 		route    *K8sRoute
// 		needSync bool
// 		status   gw.RouteStatus
// 		expected gw.RouteStatus
// 	}{{
// 		name:     "same-httproute",
// 		needSync: false,
// 		route: &K8sRoute{
// 			Route: &gw.HTTPRoute{
// 				Status: gw.HTTPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status:   status,
// 		expected: status,
// 	}, {
// 		name:     "same-tlsroute",
// 		needSync: false,
// 		route: &K8sRoute{
// 			Route: &gw.TLSRoute{
// 				Status: gw.TLSRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status:   status,
// 		expected: status,
// 	}, {
// 		name:     "same-tcproute",
// 		needSync: false,
// 		route: &K8sRoute{
// 			Route: &gw.TCPRoute{
// 				Status: gw.TCPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status:   status,
// 		expected: status,
// 	}, {
// 		name:     "same-udproute",
// 		needSync: false,
// 		route: &K8sRoute{
// 			Route: &gw.UDPRoute{
// 				Status: gw.UDPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status:   status,
// 		expected: status,
// 	}, {
// 		name:     "swap",
// 		needSync: false,
// 		route: &K8sRoute{
// 			Route: &gw.HTTPRoute{
// 				Status: gw.HTTPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status: gw.RouteStatus{
// 			Parents: append([]gw.RouteParentStatus{status.Parents[lastIndex]}, status.Parents[0:lastIndex]...),
// 		},
// 		expected: status,
// 	}, {
// 		name:     "drop",
// 		needSync: true,
// 		route: &K8sRoute{
// 			Route: &gw.HTTPRoute{
// 				Status: gw.HTTPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status: gw.RouteStatus{
// 			Parents: status.Parents[0:lastIndex],
// 		},
// 		expected: gw.RouteStatus{
// 			Parents: status.Parents[0:lastIndex],
// 		},
// 	}, {
// 		name:     "change",
// 		needSync: true,
// 		route: &K8sRoute{
// 			Route: &gw.HTTPRoute{
// 				Status: gw.HTTPRouteStatus{
// 					RouteStatus: status,
// 				},
// 			},
// 			needsStatusSync: false,
// 		},
// 		status: gw.RouteStatus{
// 			Parents: append([]gw.RouteParentStatus{status.Parents[1], status.Parents[1]}, status.Parents[1:lastIndex]...),
// 		},
// 		expected: gw.RouteStatus{
// 			Parents: append([]gw.RouteParentStatus{status.Parents[1], status.Parents[1]}, status.Parents[1:lastIndex]...),
// 		},
// 	}} {
// 		t.Run(test.name, func(t *testing.T) {
// 			test.route.setStatus(test.status)
// 			require.Equal(t, test.expected, test.route.RouteStatus())
// 			require.Equal(t, test.needSync, test.route.needsStatusSync)
// 		})
// 	}
// }
