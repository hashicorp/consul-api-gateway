package certificates

import (
	"context"
	"errors"
	// "fmt"
	"net"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul-api-gateway/internal/consul/certificates/cache"
	"github.com/hashicorp/consul-api-gateway/internal/consul/certificates/certuri"
	"github.com/hashicorp/consul-api-gateway/internal/consul/certificates/csr"
	"github.com/hashicorp/consul-api-gateway/internal/consul/certificates/utils"

	"github.com/hashicorp/consul/proto-public/pbconnectca"
)

// Fetch is a caching interface to get the current leaf certificate for a Consul
// service, generating a new private key and certificate signing request if
// necessary. Result argument represents the last result currently in cache if any
// along with its state.
// func Fetch(req *ConnectCALeafRequest, result cache.FetchResult) (cache.FetchResult, error) {
// 	/*
// 		var state fetchState
// 		if result.State != nil {
// 			var ok bool
// 			state, ok = result.State.(fetchState)
// 			if !ok {
// 				return result, fmt.Errorf(
// 					"Internal cache failure: result state wrong type: %T", result.State)
// 			}
// 		}
// 	*/

// 	// Need to lookup RootCAs response to discover trust domain. This should be a
// 	// cache hit.
// 	roots, err := c.rootsFromCache()
// 	if err != nil {
// 		return result, err
// 	}
// 	if roots.TrustDomain == "" {
// 		return result, errors.New("cluster has no CA bootstrapped yet")
// 	}

// 	rsp, err := GenerateNewLeaf(&certuri.SpiffeIDService{
// 		Host:       roots.trustDomain,
// 		Datacenter: req.Datacenter,
// 		Partition:  req.TargetPartition(),
// 		Namespace:  req.TargetNamespace(),
// 		Service:    req.Service,
// 	}, req.DNSSAN, req.IPAddresses)

// 	if gerr, ok := status.FromError(err); !ok {
// 		if gerr.Code() == codes.ResourceExhausted {
// 			if result.Value == nil {
// 				// This was a first fetch - we have no good value in cache. In this case
// 				// we just return the error to the caller rather than rely on surprising
// 				// semi-blocking until the rate limit is appeased or we timeout
// 				// behavior. It's likely the caller isn't expecting this to block since
// 				// it's an initial fetch. This also massively simplifies this edge case.
// 				return result, err
// 			}

// 			if state.activeRootRotationStart.IsZero() {
// 				// We hit a rate limit error by chance - for example a cert expired
// 				// before the root rotation was observed (not triggered by rotation) but
// 				// while server is working through high load from a recent rotation.
// 				// Just pretend there is a rotation and the retry logic here will start
// 				// jittering and retrying in the same way from now.
// 				state.activeRootRotationStart = time.Now()
// 			}

// 			// Increment the errors in the state
// 			state.consecutiveRateLimitErrs++

// 			delay := utils.RandomStagger(caChangeJitterWindow)
// 			if c.TestOverrideCAChangeInitialDelay > 0 {
// 				delay = c.TestOverrideCAChangeInitialDelay
// 			}

// 			// Find the start of the next window we can retry in. See comment on
// 			// caChangeJitterWindow for details of why we use this strategy.
// 			windowStart := state.activeRootRotationStart.Add(
// 				time.Duration(state.consecutiveRateLimitErrs) * delay)

// 			// Pick a random time in that window
// 			state.forceExpireAfter = windowStart.Add(delay)

// 			// Return a result with the existing cert but the new state - the cache
// 			// will see this as no change. Note that we always have an existing result
// 			// here due to the nil value check above.
// 			result.State = state
// 			return result, nil
// 		}

// 		return result, gerr.Err()
// 	}

// 	// TODO: Reset rotation state
// 	// state.forceExpireAfter = time.Time{}
// 	// state.consecutiveRateLimitErrs = 0
// 	// state.activeRootRotationStart = time.Time{}

// 	cert, err := csr.ParseCert(rsp.CertPem)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// TODO: Set the CA key ID so we can easily tell when a active root has changed.
// 	// state.authorityKeyID = csr.EncodeSigningKeyID(cert.AuthorityKeyId)

// 	/*
// 		result.Value = &reply
// 		// Store value not pointer so we don't accidentally mutate the cache entry
// 		// state in Fetch.
// 		result.State = state
// 		result.Index = reply.ModifyIndex
// 		return result, nil
// 	*/

// 	return cert, err
// }

// GenerateNewLeaf does the actual work of creating a new private key,
// generating a CSR and getting it signed by the servers.
func GenerateNewLeaf(id certuri.CertURI, dnsNames []string, ipAddresses []net.IP) (*pbconnectca.SignResponse, error) {
	// Create a new private key
	//
	// TODO: for now we always generate EC keys on clients regardless of the key
	// type being used by the active CA. This is fine and allowed in TLS1.2 and
	// signing EC CSRs with an RSA key is supported by all current CA providers so
	// it's OK. IF we ever need to support a CA provider that refuses to sign a
	// CSR with a different signature algorithm, or if we have compatibility
	// issues with external PKI systems that require EC certs be signed with ECDSA
	// from the CA (this was required in TLS1.1 but not in 1.2) then we can
	// instead intelligently pick the key type we generate here based on the key
	// type of the active signing CA. We already have that loaded since we need
	// the trust domain.
	pk, _, err := csr.GeneratePrivateKeyWithConfig("ec", 256)
	if err != nil {
		return nil, err
	}

	uris := []*url.URL{id.URI()}

	// Create a CSR.
	csr, err := csr.CreateCSR(uris, pk, dnsNames, ipAddresses)
	if err != nil {
		return nil, err
	}

	// TODO: dial Consul gRPC server address
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	// Create new gRPC client
	client := pbconnectca.NewConnectCAServiceClient(conn)

	// Ask server to sign certificate signing request
	rsp, err := client.Sign(context.Background(), &pbconnectca.SignRequest{
		Csr: csr,
	})
	if err != nil {
		return nil, err
	}

	return rsp, nil
}
