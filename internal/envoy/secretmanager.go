package envoy

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/protobuf/types/known/anypb"
)

type SecretClient interface {
	FetchSecret(name string) (time.Time, Certificate, error)
}

type stubSecretClient struct{}

func (s *stubSecretClient) FetchSecret(name string) (time.Time, Certificate, error) {
	return time.Now(), Certificate{}, nil
}

type Certificate struct {
	Name             string
	CertificateChain []byte
	PrivateKey       []byte
}

type persistedCertificate struct {
	Certificate
	expiresAt time.Time
	path      string
	refs      int64
}

type SecretManager struct {
	directory     string
	client        SecretClient
	watchers      map[int64]map[string]struct{}
	registry      map[string]*persistedCertificate
	cache         *cache.LinearCache
	watcherMutex  sync.RWMutex
	registryMutex sync.RWMutex
	logger        hclog.Logger
}

func NewSecretManager(client SecretClient, directory string, cache *cache.LinearCache, logger hclog.Logger) *SecretManager {
	return &SecretManager{
		client:    client,
		directory: directory,
		cache:     cache,
		logger:    logger,
		watchers:  make(map[int64]map[string]struct{}),
		registry:  make(map[string]*persistedCertificate),
	}
}

func (s *SecretManager) Watch(name string, streamID int64) error {
	maybeInsertSecret := func() error {
		s.registryMutex.Lock()
		defer s.registryMutex.Unlock()
		if entry, ok := s.registry[name]; ok {
			atomic.AddInt64(&entry.refs, 1)
			return nil
		}
		expiration, certificate, err := s.client.FetchSecret(name)
		if err != nil {
			return err
		}

		path, err := s.persist(certificate, name)
		if err != nil {
			return err
		}

		s.registry[name] = &persistedCertificate{
			Certificate: certificate,
			expiresAt:   expiration,
			path:        path,
			refs:        1,
		}
		return nil
	}

	s.watcherMutex.Lock()
	defer s.watcherMutex.Unlock()
	if watcher, ok := s.watchers[streamID]; ok {
		if _, found := watcher[name]; found {
			// already watching
			return nil
		}
		watcher[name] = struct{}{}
		return maybeInsertSecret()
	}
	s.watchers[streamID] = map[string]struct{}{
		name: {},
	}
	return maybeInsertSecret()
}

func (s *SecretManager) UnwatchAll(streamID int64) {
	s.watcherMutex.Lock()
	defer s.watcherMutex.Unlock()
	if watcher, ok := s.watchers[streamID]; ok {
		s.registryMutex.Lock()
		defer s.registryMutex.Unlock()
		for name := range watcher {
			if certificate, ok := s.registry[name]; ok {
				if atomic.AddInt64(&certificate.refs, -1) == 0 {
					// we have no more references, GC
					delete(s.registry, name)
					s.deleteCertificate(name)
				}
			}
		}
		delete(s.watchers, streamID)
	}
}

func (s *SecretManager) Unwatch(name string, streamID int64) {
	s.watcherMutex.Lock()
	defer s.watcherMutex.Unlock()
	if watcher, ok := s.watchers[streamID]; ok {
		if _, ok := watcher[name]; !ok {
			return
		}
		delete(watcher, name)

		s.registryMutex.Lock()
		defer s.registryMutex.Unlock()
		if certificate, ok := s.registry[name]; ok {
			if atomic.AddInt64(&certificate.refs, -1) == 0 {
				// we have no more references, GC
				delete(s.registry, name)
				s.deleteCertificate(name)
			}
		}
	}
}

func (s *SecretManager) Manage(ctx context.Context) {
	for {
		select {
		case <-time.After(30 * time.Second):
			s.registryMutex.RLock()
			for secretName, secret := range s.registry {
				if time.Now().After(secret.expiresAt.Add(-10 * time.Minute)) {
					// request secret and persist
					expiration, certificate, err := s.client.FetchSecret(secretName)
					if err != nil {
						s.logger.Error("error updating secret", "error", err, "secret", secretName)
						s.registryMutex.RUnlock()
						continue
					}
					_, err = s.persist(certificate, secretName)
					if err != nil {
						s.logger.Error("error persisting secret", "error", err, "secret", secretName)
						s.registryMutex.RUnlock()
						continue
					}
					secret.Certificate = certificate
					secret.expiresAt = expiration
				}
			}
			s.registryMutex.RUnlock()
		case <-ctx.Done():
			// we finished the context, just return
			return
		}
	}
}

func (s *SecretManager) persist(certificate Certificate, name string) (string, error) {
	file := path.Join(s.directory, name)
	data, err := json.Marshal(&certificate)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		return "", err
	}
	// file write succeeded, persist cert in the cache
	if err := s.updateCertificate(certificate); err != nil {
		return "", err
	}
	return file, nil
}

func (s *SecretManager) updateCertificate(c Certificate) error {
	resource, err := anypb.New(&tls.Secret{
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
	})
	if err != nil {
		return err
	}
	return s.cache.UpdateResource(c.Name, &discovery.Resource{
		Name:     c.Name,
		Resource: resource,
	})
}

func (s *SecretManager) deleteCertificate(name string) error {
	return s.cache.DeleteResource(name)
}
