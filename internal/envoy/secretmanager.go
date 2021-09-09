package envoy

import (
	"context"
	"sync"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/hashicorp/go-hclog"
)

type SecretClient interface {
	FetchSecret(ctx context.Context, name string) (*Certificate, error)
}

type Certificate struct {
	Name             string
	ExpiresAt        time.Time
	CertificateChain []byte
	PrivateKey       []byte
}

// reference counted Certificates
type watchedCertificate struct {
	*Certificate
	refs map[string]struct{}
}

type SecretManager struct {
	client SecretClient
	// map to contain sets of cert names that a stream is watching
	watchers map[string]map[string]struct{}
	// map of cert names to certs
	registry map[string]*watchedCertificate
	cache    *cache.LinearCache
	mutex    sync.RWMutex
	logger   hclog.Logger
}

func NewSecretManager(client SecretClient, cache *cache.LinearCache, logger hclog.Logger) *SecretManager {
	return &SecretManager{
		client:   client,
		cache:    cache,
		logger:   logger,
		watchers: make(map[string]map[string]struct{}),
		registry: make(map[string]*watchedCertificate),
	}
}

func (s *SecretManager) Watch(ctx context.Context, names []string, node string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	watcher, ok := s.watchers[node]
	if !ok {
		watcher = map[string]struct{}{}
		s.watchers[node] = watcher
	}

	certificates := []*Certificate{}
	for _, name := range names {
		watcher[name] = struct{}{}
		if entry, ok := s.registry[name]; ok {
			certificates = append(certificates, entry.Certificate)
			entry.refs[node] = struct{}{}
			continue
		}
		certificate, err := s.client.FetchSecret(ctx, name)
		if err != nil {
			return err
		}
		certificates = append(certificates, certificate)
		s.registry[name] = &watchedCertificate{
			Certificate: certificate,
			refs: map[string]struct{}{
				node: {},
			},
		}
	}
	err := s.updateCertificates(ctx, certificates)
	if err != nil {
		return err
	}
	return nil
}

func (s *SecretManager) UnwatchAll(ctx context.Context, node string) {
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

func (s *SecretManager) Unwatch(ctx context.Context, names []string, node string) error {
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

func (s *SecretManager) Manage(ctx context.Context) {
	s.logger.Trace("running secrets manager")

	for {
		select {
		case <-time.After(30 * time.Second):
			s.mutex.RLock()
			for secretName, secret := range s.registry {
				if time.Now().After(secret.ExpiresAt.Add(-10 * time.Minute)) {
					// request secret and persist
					certificate, err := s.client.FetchSecret(ctx, secretName)
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
					secret.Certificate = certificate
				}
			}
			s.mutex.RUnlock()
		case <-ctx.Done():
			// we finished the context, just return
			return
		}
	}
}

func certToSecret(c *Certificate) types.Resource {
	return &tls.Secret{
		Type: &tls.Secret_TlsCertificate{
			TlsCertificate: &tls.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: c.CertificateChain,
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: c.PrivateKey,
					},
				},
			},
		},
		Name: c.Name,
	}
}

func (s *SecretManager) updateCertificate(ctx context.Context, c *Certificate) error {
	return s.cache.UpdateResource(c.Name, certToSecret(c))
}

func (s *SecretManager) updateCertificates(ctx context.Context, certs []*Certificate) error {
	for _, cert := range certs {
		if err := s.updateCertificate(ctx, cert); err != nil {
			return err
		}
	}
	return nil
}

func (s *SecretManager) removeCertificates(ctx context.Context, names []string) error {
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
