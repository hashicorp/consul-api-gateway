---
name: Bug Report
about: You're experiencing an issue with the Consul API Gateway that is different than the documented behavior.
labels: bug

---

<!--- Please keep this note for the community --->

### Community Note

* Please vote on this issue by adding a 👍 [reaction](https://blog.github.com/2016-03-10-add-reactions-to-pull-requests-issues-and-comments/) to the original issue to help the community and maintainers prioritize this request. Searching for pre-existing feature requests helps us consolidate datapoints for identical requirements into a single place, thank you!
* Please do not leave "+1" or other comments that do not add relevant new information or questions, they generate extra noise for issue followers and do not help prioritize the request.
* If you are interested in working on this issue or have submitted a pull request, please leave a comment.

<!--- Thank you for keeping this note for the community --->

---

<!--- When filing a bug, please include the following headings if possible. Any example text in this template can be deleted. --->

### Overview of the Issue

<!--- Please describe the issue you are having and how you encountered the problem. --->

### Reproduction Steps

<!--- 

In order to effectively and quickly resolve the issue, please provide exact steps that allow us the reproduce the problem. If no steps are provided, then it will likely take longer to get the issue resolved. An example that you can follow is provided below. 

Steps to reproduce this issue, eg:

1. When creating a gateway with the following configuration:
```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
...
```
1. View error

  --->

### Logs

<!---

Provide log files from the gateway controller component by providing output from `kubectl logs` from the pod and container that is surfacing the issue. 

<details>
  <summary>Logs</summary>

```
output from 'kubectl logs':
```

</details>

--->

### Expected behavior

<!--- What was the expected result after following the reproduction steps? --->

### Environment details

<!---

If not already included, please provide the following:
- `consul-api-gateway` version:
- configuration used to deploy the gateway controller:

Additionally, please provide details regarding the Kubernetes Infrastructure, as shown below:
- Kubernetes version: v1.22.x
- Consul Server version: v1.11.x
- Consul-K8s version
- Cloud Provider (If self-hosted, the Kubernetes provider utilized): EKS, AKS, GKE, OpenShift (and version), Rancher (and version), TKGI (and version)
- Networking CNI plugin in use: Calico, Cilium, NSX-T 

Any other information you can provide about the environment/deployment.
--->


### Additional Context

<!---
Additional context on the problem. Docs, links to blogs, or other material that lead you to discover this issue or were helpful in troubleshooting the issue. 
--->
