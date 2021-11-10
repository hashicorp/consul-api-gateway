# Contributing

> **Note:** We take security and our users' trust very seriously.
> If you believe you have found a security issue, please responsibly
> disclose by following the steps in our [security policy](https://github.com/hashicorp/consul-api-gateway/security/policy).

**First:** if you're unsure or afraid of _anything_, just ask or submit the
issue or pull request anyways. You won't be yelled at for giving your best
effort. The worst that can happen is that you'll be politely asked to change
something. We appreciate any sort of contributions, and don't want a wall of
rules to get in the way of that.

That said, if you want to ensure that a pull request is likely to be merged, 
talk to us! A great way to do this is in issues themselves. When you want to 
work on an issue, comment on it first and tell us the approach you want to take.

## Reporting an Issue

> **Note:** Please see [`SUPPORT.md`](./SUPPORT.md) for guidance on where to 
> ask product questions or how to get help.

* Make sure you test against the latest released version. It is possible we 
already fixed the bug you're experiencing. However, if you are on an older 
version and feel the issue is critical, do let us know.

* Check existing issues (both open and closed) to make sure it has not been 
reported previously.

* Provide a reproducible test case. If a contributor can't reproduce an issue, 
then it dramatically lowers the chances it'll get fixed.

* Aim to respond promptly to any questions made by the maintainers on your 
issue. Stale issues may be closed.

## Developing

If you make any changes to the code, please run `make fmt` to automatically format the code according to Go standards.
If a dependency is added or changed, please run `go mod tidy` to update `go.mod` and `go.sum`.

When opening your first pull request to a HashiCorp project, you may be asked to sign a Contributor Licensing Agreement before your contribution can be merged.

### Developer Documentation

A quick start guide, sample deployment instructions and entity relationship diagrams are available in our [developer documentation](../dev/docs).

## Testing

During development, it may be more convenient to check your work-in-progress by running only the tests which you expect to be affected by your changes.
[Go's built-in test tool](https://golang.org/pkg/cmd/go/internal/test/) allows specifying a list of packages to test and the `-run` option to only include test names matching a regular expression.

When a pull request is opened, CI will lint syntax and run the full test suite.
