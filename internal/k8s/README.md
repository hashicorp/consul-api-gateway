# Design

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
4. In order to facillitate the above, wrap all controller `Reconcile` methods in `NewSyncRequeueingMiddleware`
and make sure any errors returned from calls to an adapter's `Sync` method are wrapped in `core.SyncError`.