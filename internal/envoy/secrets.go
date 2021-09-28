package envoy

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"

	"github.com/hashicorp/go-hclog"
)

//go:generate mockgen -source ./secrets.go -destination ./mocks/secrets.go -package mocks SecretManager,SecretClient,SecretCache

var ErrInvalidSecretProtocol = errors.New("secret protocol is not registered")

// SecretCache is used as an intermediate cache for pushing tls certificates into. In practice
// we're using github.com/envoyproxy/go-control-plane/pkg/cache.(*LinearCache) as the concrete
// struct that implements this and handles notifying watched gRPC streams when we push new
// requested resources into into or delete existing resources from the cache.
type SecretCache interface {
	UpdateResource(name string, res types.Resource) error
	DeleteResource(name string) error
}

// SecretClient is used to retrieve TLS secrets. When a gRPC stream attempts to watch a secret
// we first check if we've already pushed it into our intermediate SecretCache, if we have, then
// we only increment a reference counter used to track the lifecycle of the watched secret. If
// we are not yet tracking the secret, we retrieve it remotely via the SecretClient.
type SecretClient interface {
	FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error)
}

// MultiSecretClient implements a registry of secret clients that handle fetching secrets
// based off of the protocol they're given in the secret name.
type MultiSecretClient struct {
	fetchers map[string]SecretClient
	mutex    sync.RWMutex
}

func NewMultiSecretClient() *MultiSecretClient {
	return &MultiSecretClient{
		fetchers: make(map[string]SecretClient),
	}
}

func (m *MultiSecretClient) Register(protocol string, client SecretClient) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.fetchers[protocol] = client
}

func (m *MultiSecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	parsed, err := url.Parse(name)
	if err != nil {
		return nil, time.Time{}, err
	}
	fetcher, found := m.fetchers[parsed.Scheme]
	if !found {
		return nil, time.Time{}, ErrInvalidSecretProtocol
	}
	return fetcher.FetchSecret(ctx, name)
}

// SecretManager handles the lifecycle of watched TLS secrets.
type SecretManager interface {
	// SetResourcesForNode sets a list of TLS certificates being tracked by the node
	SetResourcesForNode(ctx context.Context, names []string, node string) error
	// Watch is used for tracking an envoy node's TLS secrets of interest
	Watch(ctx context.Context, names []string, node string) error
	// Unwatch is used for removing a subset of an envoy node's TLS secrets
	// from the list the node's secrets of interest
	Unwatch(ctx context.Context, names []string, node string) error
	// UnwatchAll is used to completely unwatch all a node's secrets
	UnwatchAll(ctx context.Context, node string) error
	// Manage is used for re-fetching expiring TLS certificates and updating them
	Manage(ctx context.Context)
}

// reference counted certificates
type watchedCertificate struct {
	*tls.Secret
	expiration time.Time
	refs       map[string]struct{}
}

type secretManager struct {
	// map to contain sets of cert names that a stream is watching
	watchers map[string]map[string]struct{}
	// map of cert names to certs
	registry map[string]*watchedCertificate
	// this mutex protects both watchers and registry as they're
	// almost always used in tandem
	mutex           sync.RWMutex
	client          SecretClient
	cache           SecretCache
	logger          hclog.Logger
	loopTimeout     time.Duration
	expirationDelta time.Duration
}

// NewSecretManager returns a secret manager that manages the use of an underlying SecretClient
// and SecretCache to track and keep TLS secrets up-to-date.
func NewSecretManager(client SecretClient, cache SecretCache, logger hclog.Logger) *secretManager {
	return &secretManager{
		client:          client,
		cache:           cache,
		logger:          logger,
		watchers:        make(map[string]map[string]struct{}),
		registry:        make(map[string]*watchedCertificate),
		loopTimeout:     30 * time.Second,
		expirationDelta: 10 * time.Minute,
	}
}

// Nodes returns a list of watched nodes
func (s *secretManager) Nodes() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	nodes := []string{}
	for node := range s.watchers {
		nodes = append(nodes, node)
	}
	return nodes
}

// Resources returns a list of TLS certificates being tracked
func (s *secretManager) Resources() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	resources := []string{}
	for resource := range s.registry {
		resources = append(resources, resource)
	}
	return resources
}

// SetResourcesForNode sets a list of TLS certificates being tracked by the node
func (s *secretManager) SetResourcesForNode(ctx context.Context, names []string, node string) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	watcher, found := s.watchers[node]
	if !found {
		return s.watch(ctx, names, node)
	}

	unwatch := []string{}
	watch := []string{}
	// fast lookups for calculating what needs to be unwatched
	keepMap := make(map[string]struct{})
	for _, resource := range names {
		if _, watched := watcher[resource]; !watched {
			watch = append(watch, resource)
		} else {
			keepMap[resource] = struct{}{}
		}
	}
	for resource := range watcher {
		if _, keep := keepMap[resource]; !keep {
			unwatch = append(unwatch, resource)
		}
	}

	if err := s.watch(ctx, watch, node); err != nil {
		return err
	}
	return s.unwatch(ctx, unwatch, node)
}

// Watch is used for tracking an envoy node's TLS secrets of interest
func (s *secretManager) Watch(ctx context.Context, names []string, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.watch(ctx, names, node)
}

// this must be called with the mutex lock held
func (s *secretManager) watch(ctx context.Context, names []string, node string) error {
	watcher, ok := s.watchers[node]
	if !ok {
		// no watcher found, initialize one
		watcher = map[string]struct{}{}
		s.watchers[node] = watcher
	}

	certificates := []*tls.Secret{}
	for _, name := range names {
		watcher[name] = struct{}{}
		if entry, ok := s.registry[name]; ok {
			// we found an entry in our TLS cert tracking map, add a reference to it
			entry.refs[node] = struct{}{}
			continue
		}
		// no existing entry, fetch the certificate
		certificate, expires, err := s.client.FetchSecret(ctx, name)
		if err != nil {
			return err
		}
		// add the certificate to our watch tracker along with an initial reference
		// list of our currently requesting node
		watchedCert := &watchedCertificate{
			Secret:     certificate,
			expiration: expires,
			refs: map[string]struct{}{
				node: {},
			},
		}
		// add the certificate to the list of those that we need to push into the cache
		certificates = append(certificates, certificate)
		s.registry[name] = watchedCert
	}
	// push all newly fetched certificates into the underlying cache
	err := s.updateCertificates(ctx, certificates)
	if err != nil {
		return err
	}
	return nil
}

// UnwatchAll is used to completely unwatch all a node's secrets
func (s *secretManager) UnwatchAll(ctx context.Context, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if watcher, ok := s.watchers[node]; ok {
		certificates := []string{}
		for name := range watcher {
			// remove the node from all of the reference lists of resources
			// it's tracking
			if certificate, ok := s.registry[name]; ok {
				delete(certificate.refs, node)
				if len(certificate.refs) == 0 {
					// we have no more references for a resource, GC it
					delete(s.registry, name)
					// mark the certificate as needing to be removed from the underlying cache
					certificates = append(certificates, name)
				}
			}
		}
		// purge the node from our tracker
		delete(s.watchers, node)
		// purge GC'd certificates from the cache
		return s.removeCertificates(ctx, certificates)
	}
	return nil
}

// Unwatch is used for removing a subset of an envoy node's TLS secrets
// from the list the node's secrets of interest
func (s *secretManager) Unwatch(ctx context.Context, names []string, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.unwatch(ctx, names, node)
}

// this must be called with the mutex lock held
func (s *secretManager) unwatch(ctx context.Context, names []string, node string) error {
	if watcher, ok := s.watchers[node]; ok {
		certificates := []string{}
		// remove the node from of requested reference lists
		for _, name := range names {
			// mark that we're no longer interested in this certificate
			delete(watcher, name)
			if certificate, ok := s.registry[name]; ok {
				// delete the node from the reference list
				delete(certificate.refs, node)
				if len(certificate.refs) == 0 {
					// we have no more references for a resource, GC it
					delete(s.registry, name)
					// mark the certificate as needing to be removed from the underlying cache
					certificates = append(certificates, name)
				}
			}
		}
		// we're not tracking any references any more, so just delete this from
		// our tracker -- if it starts watching resources again, it will get added
		// back in
		if len(watcher) == 0 {
			delete(s.watchers, node)
		}
		// purge GC'd certificates from the cache
		return s.removeCertificates(ctx, certificates)
	}
	return nil
}

// Manage is used for re-fetching expiring TLS certificates and updating them
func (s *secretManager) Manage(ctx context.Context) {
	s.logger.Trace("running secrets manager")

	for {
		select {
		case <-time.After(s.loopTimeout):
			s.manage(ctx)
		case <-ctx.Done():
			// we finished the context, just return
			return
		}
	}
}

func (s *secretManager) manage(ctx context.Context) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for secretName, secret := range s.registry {
		// check the certificate to see if we're within a window close to its
		// expiration, when we want to start re-fetching it because of it
		// potentially getting re-issued
		if time.Now().After(secret.expiration.Add(-s.expirationDelta)) {
			// fetch the certificate and add it to the cache
			certificate, expires, err := s.client.FetchSecret(ctx, secretName)
			if err != nil {
				s.logger.Error("error fetching secret", "error", err, "secret", secretName)
				continue
			}
			err = s.updateCertificate(ctx, certificate)
			if err != nil {
				s.logger.Error("error updating secret", "error", err, "secret", secretName)
				continue
			}
			// since we don't want to lose the referenced nodes, just update individual
			// fields on the tracking struct
			secret.Secret = certificate
			secret.expiration = expires
		}
	}
}

func (s *secretManager) updateCertificate(ctx context.Context, c *tls.Secret) error {
	return s.cache.UpdateResource(c.Name, c)
}

func (s *secretManager) updateCertificates(ctx context.Context, certs []*tls.Secret) error {
	for _, cert := range certs {
		if err := s.updateCertificate(ctx, cert); err != nil {
			return err
		}
	}
	return nil
}

func (s *secretManager) removeCertificates(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	for _, name := range names {
		s.logger.Debug("removing resource", "name", name)
		if err := s.cache.DeleteResource(name); err != nil {
			return err
		}
	}
	return nil
}
