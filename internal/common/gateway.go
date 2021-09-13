package common

import (
	"sync"
)

type GatewayInfo struct {
	Namespace string
	Service   string
}

type GatewayRegistry struct {
	secrets map[GatewayInfo]map[string]struct{}
	mutex   sync.RWMutex
}

func NewGatewayRegistry() *GatewayRegistry {
	return &GatewayRegistry{
		secrets: make(map[GatewayInfo]map[string]struct{}),
	}
}

func (r *GatewayRegistry) GatewayExists(info *GatewayInfo) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, found := r.secrets[*info]
	return found
}

func (r *GatewayRegistry) CanFetchSecrets(info *GatewayInfo, secrets []string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	storedSecrets := r.secrets[*info]
	for _, secret := range secrets {
		if _, found := storedSecrets[secret]; !found {
			return false
		}
	}
	return true
}

func (r *GatewayRegistry) AddGateway(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets := make(map[string]struct{})
	for _, secret := range secrets {
		storedSecrets[secret] = struct{}{}
	}
	r.secrets[info] = storedSecrets
}

func (r *GatewayRegistry) AddSecrets(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets := r.secrets[info]
	for _, secret := range secrets {
		storedSecrets[secret] = struct{}{}
	}
}

func (r *GatewayRegistry) RemoveGateway(info GatewayInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.secrets, info)
}

func (r *GatewayRegistry) RemoveSecrets(info GatewayInfo, secrets ...string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	storedSecrets := r.secrets[info]
	for _, secret := range secrets {
		delete(storedSecrets, secret)
	}
}
