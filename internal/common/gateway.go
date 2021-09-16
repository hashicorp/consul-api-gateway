package common

import (
	"sync"
)

// GatewayInfo encapsulates enough information
// to describe a particular deployed gateway
type GatewayInfo struct {
	Namespace string
	Service   string
}

// GatewaySecretRegistry is the source we can use to
// lookup what gateways are deployed and what secrets
// they have access to
type GatewaySecretRegistry struct {
	secrets map[GatewayInfo]map[string]struct{}
	mutex   sync.RWMutex
}

// NewGatewaySecretRegistry initializes a GatewaySecretRegistry instance
func NewGatewaySecretRegistry() *GatewaySecretRegistry {
	return &GatewaySecretRegistry{
		secrets: make(map[GatewayInfo]map[string]struct{}),
	}
}

// GatewayExists checks if the registry knows about a gateway
func (r *GatewaySecretRegistry) GatewayExists(info GatewayInfo) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, found := r.secrets[info]
	return found
}

// CanFetchSecrets checks if a gateway should be able to access a set of secrets
func (r *GatewaySecretRegistry) CanFetchSecrets(info GatewayInfo, secrets []string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	storedSecrets, ok := r.secrets[info]
	if !ok {
		return false
	}
	for _, secret := range secrets {
		if _, found := storedSecrets[secret]; !found {
			return false
		}
	}
	return true
}

// AddGateway adds a gateway to the registry with a set of secrets that it can access
func (r *GatewaySecretRegistry) AddGateway(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets := make(map[string]struct{})
	for _, secret := range secrets {
		storedSecrets[secret] = struct{}{}
	}
	r.secrets[info] = storedSecrets
}

// AddSecrets adds a set of secrets that the given gateway has access to
func (r *GatewaySecretRegistry) AddSecrets(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets, ok := r.secrets[info]
	if ok {
		for _, secret := range secrets {
			storedSecrets[secret] = struct{}{}
		}
	}
}

// RemoveGateway removes a gateway from the registry
func (r *GatewaySecretRegistry) RemoveGateway(info GatewayInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.secrets, info)
}

// RemoveSecrets removes a set of secrets that gateway should no longer have access to
func (r *GatewaySecretRegistry) RemoveSecrets(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets, ok := r.secrets[info]
	if ok {
		for _, secret := range secrets {
			delete(storedSecrets, secret)
		}
	}
}
