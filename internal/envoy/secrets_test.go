package envoy

import (
	"context"
	"errors"
	"testing"
	"time"

	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/polar/internal/envoy/mocks"
	"github.com/stretchr/testify/require"
)

func TestWatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	secretClient := mocks.NewMockSecretClient(ctrl)
	cache := mocks.NewMockSecretCache(ctrl)

	secretOne := &tls.Secret{
		Name: "one",
	}
	secretTwo := &tls.Secret{
		Name: "two",
	}
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretOne.Name).Return(secretOne, time.Now(), nil)
	cache.EXPECT().UpdateResource(secretOne.Name, secretOne)
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretTwo.Name).Return(secretTwo, time.Now(), nil)
	cache.EXPECT().UpdateResource(secretTwo.Name, secretTwo)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secretOne.Name, secretTwo.Name, secretOne.Name}, "node")
	require.NoError(t, err)
	// call again and everything should hit cache
	err = manager.Watch(context.Background(), []string{secretOne.Name, secretTwo.Name, secretOne.Name}, "node")
	require.NoError(t, err)
}

func TestWatch_FetchError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	secretClient := mocks.NewMockSecretClient(ctrl)
	cache := mocks.NewMockSecretCache(ctrl)

	secretName := "one"
	fetchError := errors.New("fetch error")
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretName).Return(nil, time.Now(), fetchError)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secretName}, "node")
	require.Equal(t, fetchError, err)
}

func TestWatch_CacheError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	secretClient := mocks.NewMockSecretClient(ctrl)
	cache := mocks.NewMockSecretCache(ctrl)

	secret := &tls.Secret{
		Name: "one",
	}
	cacheError := errors.New("cache error")
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now(), nil)
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(cacheError)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secret.Name}, "node")
	require.Equal(t, cacheError, err)
}
