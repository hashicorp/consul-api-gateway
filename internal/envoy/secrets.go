package envoy

import (
	"context"
	"sync"
	"time"

	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"

	"github.com/hashicorp/go-hclog"
)

//go:generate mockgen -source ./secrets.go -destination ./mocks/secrets.go -package mocks SecretManager,SecretClient,SecretCache

type SecretCache interface {
	UpdateResource(name string, res types.Resource) error
	DeleteResource(name string) error
}

type SecretClient interface {
	FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error)
}

type SecretManager interface {
	Watch(ctx context.Context, names []string, node string) error
	Unwatch(ctx context.Context, names []string, node string) error
	UnwatchAll(ctx context.Context, node string)
	Manage(ctx context.Context)
}

// reference counted Certificates
type watchedCertificate struct {
	*tls.Secret
	expiration time.Time
	refs       map[string]struct{}
}

type secretManager struct {
	client SecretClient
	// map to contain sets of cert names that a stream is watching
	watchers map[string]map[string]struct{}
	// map of cert names to certs
	registry        map[string]*watchedCertificate
	cache           SecretCache
	mutex           sync.RWMutex
	logger          hclog.Logger
	loopTimeout     time.Duration
	expirationDelta time.Duration
}

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

func (s *secretManager) Nodes() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	nodes := []string{}
	for node := range s.watchers {
		nodes = append(nodes, node)
	}
	return nodes
}

func (s *secretManager) Resources() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	resources := []string{}
	for resource := range s.registry {
		resources = append(resources, resource)
	}
	return resources
}

func (s *secretManager) Watch(ctx context.Context, names []string, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	watcher, ok := s.watchers[node]
	if !ok {
		watcher = map[string]struct{}{}
		s.watchers[node] = watcher
	}

	certificates := []*tls.Secret{}
	for _, name := range names {
		watcher[name] = struct{}{}
		if entry, ok := s.registry[name]; ok {
			entry.refs[node] = struct{}{}
			continue
		}
		certificate, expires, err := s.client.FetchSecret(ctx, name)
		if err != nil {
			return err
		}
		watchedCert := &watchedCertificate{
			Secret:     certificate,
			expiration: expires,
			refs: map[string]struct{}{
				node: {},
			},
		}
		certificates = append(certificates, certificate)
		s.registry[name] = watchedCert
	}
	err := s.updateCertificates(ctx, certificates)
	if err != nil {
		return err
	}
	return nil
}

func (s *secretManager) UnwatchAll(ctx context.Context, node string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if watcher, ok := s.watchers[node]; ok {
		certificates := []string{}
		for name := range watcher {
			if certificate, ok := s.registry[name]; ok {
				delete(certificate.refs, node)
				if len(certificate.refs) == 0 {
					// we have no more references, GC
					delete(s.registry, name)
					certificates = append(certificates, name)
				}
			}
		}
		s.removeCertificates(ctx, certificates)
		delete(s.watchers, node)
	}
}

func (s *secretManager) Unwatch(ctx context.Context, names []string, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if watcher, ok := s.watchers[node]; ok {
		for _, name := range names {
			delete(watcher, name)
			if certificate, ok := s.registry[name]; ok {
				delete(certificate.refs, node)
				if len(certificate.refs) == 0 {
					// we have no more references, GC
					delete(s.registry, name)
				}
			}
		}
	}
	return s.removeCertificates(ctx, names)
}

func (s *secretManager) Manage(ctx context.Context) {
	s.logger.Trace("running secrets manager")

	for {
		select {
		case <-time.After(s.loopTimeout):
			s.mutex.RLock()
			for secretName, secret := range s.registry {
				if time.Now().After(secret.expiration.Add(-s.expirationDelta)) {
					// request secret and persist
					certificate, expires, err := s.client.FetchSecret(ctx, secretName)
					if err != nil {
						s.logger.Error("error fetching secret", "error", err, "secret", secretName)
						s.mutex.RUnlock()
						continue
					}
					err = s.updateCertificate(ctx, certificate)
					if err != nil {
						s.logger.Error("error updating secret", "error", err, "secret", secretName)
						s.mutex.RUnlock()
						continue
					}
					secret.Secret = certificate
					secret.expiration = expires
				}
			}
			s.mutex.RUnlock()
		case <-ctx.Done():
			// we finished the context, just return
			return
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
