package envoy

import (
	"context"
	"errors"
	"testing"
	"time"

	tls "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul-api-gateway/internal/envoy/mocks"
	"github.com/hashicorp/go-hclog"
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
	secretThree := &tls.Secret{
		Name: "three",
	}
	nodeOne := "nodeOne"
	nodeTwo := "nodeTwo"
	nodeThree := "nodeThree"
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretOne.Name).Return(secretOne, time.Now(), nil)
	cache.EXPECT().UpdateResource(secretOne.Name, secretOne)
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretTwo.Name).Return(secretTwo, time.Now(), nil)
	cache.EXPECT().UpdateResource(secretTwo.Name, secretTwo)
	secretClient.EXPECT().FetchSecret(gomock.Any(), secretThree.Name).Return(secretThree, time.Now(), nil)
	cache.EXPECT().UpdateResource(secretThree.Name, secretThree)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secretOne.Name, secretTwo.Name, secretOne.Name}, nodeOne)
	require.NoError(t, err)
	// call again and everything should hit cache
	err = manager.Watch(context.Background(), []string{secretTwo.Name, secretThree.Name}, nodeTwo)
	require.NoError(t, err)
	err = manager.Watch(context.Background(), []string{secretThree.Name, secretOne.Name}, nodeThree)
	require.NoError(t, err)
	nodes := manager.Nodes()
	secrets := manager.Resources()
	require.Len(t, nodes, 3)
	require.Len(t, secrets, 3)
	require.ElementsMatch(t, []string{nodeOne, nodeTwo, nodeThree}, nodes)
	require.ElementsMatch(t, []string{secretOne.Name, secretTwo.Name, secretThree.Name}, secrets)

	// unwatch

	// no change, disassociate from node 3
	err = manager.Unwatch(context.Background(), []string{secretOne.Name}, nodeThree)
	require.NoError(t, err)
	nodes = manager.Nodes()
	secrets = manager.Resources()
	require.Len(t, nodes, 3)
	require.Len(t, secrets, 3)
	require.ElementsMatch(t, []string{nodeOne, nodeTwo, nodeThree}, nodes)
	require.ElementsMatch(t, []string{secretOne.Name, secretTwo.Name, secretThree.Name}, secrets)

	// no change in secrets, purge node 3
	err = manager.Unwatch(context.Background(), []string{secretThree.Name}, nodeThree)
	require.NoError(t, err)
	nodes = manager.Nodes()
	secrets = manager.Resources()
	require.Len(t, nodes, 2)
	require.Len(t, secrets, 3)
	require.ElementsMatch(t, []string{nodeOne, nodeTwo}, nodes)
	require.ElementsMatch(t, []string{secretOne.Name, secretTwo.Name, secretThree.Name}, secrets)

	// unwatch untracked node
	err = manager.Unwatch(context.Background(), []string{secretThree.Name}, nodeThree)
	require.NoError(t, err)

	// unwatch all

	// purge node 2, delete secret 3 from cache
	cache.EXPECT().DeleteResource(secretThree.Name)
	err = manager.UnwatchAll(context.Background(), nodeTwo)
	require.NoError(t, err)
	nodes = manager.Nodes()
	secrets = manager.Resources()
	require.Len(t, nodes, 1)
	require.Len(t, secrets, 2)
	require.ElementsMatch(t, []string{nodeOne}, nodes)
	require.ElementsMatch(t, []string{secretOne.Name, secretTwo.Name}, secrets)

	// unwatch untracked node
	err = manager.UnwatchAll(context.Background(), nodeTwo)
	require.NoError(t, err)

	// purge node 2, delete secret 2 from cache
	cache.EXPECT().DeleteResource(secretTwo.Name)
	err = manager.Unwatch(context.Background(), []string{secretTwo.Name}, nodeOne)
	require.NoError(t, err)
	nodes = manager.Nodes()
	secrets = manager.Resources()
	require.Len(t, nodes, 1)
	require.Len(t, secrets, 1)
	require.ElementsMatch(t, []string{nodeOne}, nodes)
	require.ElementsMatch(t, []string{secretOne.Name}, secrets)
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

func TestUnwatch_CacheError(t *testing.T) {
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
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(nil)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secret.Name}, "node")
	require.NoError(t, err)

	cache.EXPECT().DeleteResource(secret.Name).Return(cacheError)
	err = manager.Unwatch(context.Background(), []string{secret.Name}, "node")
	require.Equal(t, cacheError, err)
}

func TestManage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	secretClient := mocks.NewMockSecretClient(ctrl)
	cache := mocks.NewMockSecretCache(ctrl)
	secret := &tls.Secret{
		Name: "one",
	}
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now().Add(-20*time.Minute), nil)
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(nil)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	err := manager.Watch(context.Background(), []string{secret.Name}, "node")
	require.NoError(t, err)

	// check fetch for expired certs
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now().Add(-20*time.Minute), nil)
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(nil)
	manager.manage(context.Background())

	// error on fetch gets swallowed
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now().Add(20*time.Minute), errors.New("fetch error"))
	manager.manage(context.Background())

	// error on update gets swallowed
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now().Add(20*time.Minute), nil)
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(errors.New("update error"))
	manager.manage(context.Background())

	// check cert still expired
	secretClient.EXPECT().FetchSecret(gomock.Any(), secret.Name).Return(secret, time.Now().Add(20*time.Minute), nil)
	cache.EXPECT().UpdateResource(secret.Name, secret).Return(nil)
	manager.manage(context.Background())

	// check that cert is now valid (i.e. no fetch calls)
	manager.manage(context.Background())
}

func TestManageLoop(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	secretClient := mocks.NewMockSecretClient(ctrl)
	cache := mocks.NewMockSecretCache(ctrl)

	manager := NewSecretManager(secretClient, cache, hclog.NewNullLogger())
	manager.loopTimeout = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	// make sure we don't block
	manager.Manage(ctx)
}

type testSecretClient struct {
	called bool
}

func (t *testSecretClient) FetchSecret(ctx context.Context, name string) (*tls.Secret, time.Time, error) {
	t.called = true
	return nil, time.Time{}, nil
}

func TestMultiSecretClient(t *testing.T) {
	secretClient := &testSecretClient{}
	multi := NewMultiSecretClient()
	multi.Register("test-protocol", secretClient)

	_, _, err := multi.FetchSecret(context.Background(), "unregistered-protocol://name")
	require.Error(t, err)
	require.Equal(t, ErrInvalidSecretProtocol, err)
	_, _, err = multi.FetchSecret(context.Background(), "test-protocol://name")
	require.NoError(t, err)
	require.True(t, secretClient.called)
}
