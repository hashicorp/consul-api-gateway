# Design

## Lifecycle

Every time we enter a reconciliation loop we do essentially the same 5 things in the following order for logical
consistency:

1. See where we are in the lifecycle of an object -- if it's being deleted, clean it up, if it needs finalizer
additions or deletions, do them -- this happens in the Controller/reconciler itself.
2. Validate and resolve any references to external objects -- this happens in the reconciliation manager.
3. Attempt to upsert the object into our state tree -- this happens in the store that the manager invokes.
4. Synchronize any changed objects from the upsert into Consul's state -- this happens in a synchronization
adapter used by the store.
5. Synchronize any status changes back to Kubernetes -- this happens as callbacks invoked by the store after
synchronization either fails or completes.

## Error Handling

Generally we've followed the following major design principles for error handling in our reconciler loop code:

1. Any unexpected Kubernetes API errors should return an error from the controller `Reconcile` method which will
immediately reschedule (in a ratelimited fashion) the event on the loop.
2. Any errors that are non-retryable in nature due to bad data i.e. validation errors, should be swallowed and
Kubernetes status conditions used to give user feedback.
3. Errors from synchronizing a resolved Gateway-Route tree, whether they're Consul network-connectivity issues
or data validation issues due to external modification or invalid Consul state should be retried, however the
reconciliation loop ***should not*** be blocked, instead the reconciliation attempt should be requeued with a
delay, allowing subsequent state changes to still be processed, causing whatever guards against out-of-date
configuration (generation checks) to eventually drop the bad event.
4. In order to facillitate the above, wrap all controller `Reconcile` methods in `gatewayclient.NewRequeueingMiddleware`
and make sure any errors returned from the `gatewayclient` api calls are wrapped in `gatewayclient.K8sError`.