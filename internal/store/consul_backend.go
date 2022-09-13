package store

import (
	"context"
	"strings"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul-api-gateway/internal/core"
)

type ConsulBackend struct {
	client *consulapi.Client

	id         string
	namespace  string
	pathPrefix string
}

var _ Backend = &ConsulBackend{}

func NewConsulBackend(id string, client *consulapi.Client, namespace, pathPrefix string) *ConsulBackend {
	return &ConsulBackend{
		id:         id,
		client:     client,
		namespace:  namespace,
		pathPrefix: pathPrefix,
	}
}

func (c *ConsulBackend) GetGateway(ctx context.Context, id core.GatewayID) ([]byte, error) {
	pair, _, err := c.client.KV().Get(c.storagePath("gateways", id.ConsulNamespace, id.Service), c.queryOptions(ctx))
	if err != nil {
		return nil, err
	}
	if pair == nil {
		return nil, ErrNotFound
	}
	return pair.Value, nil
}

func (c *ConsulBackend) ListGateways(ctx context.Context) ([][]byte, error) {
	pairs, _, err := c.client.KV().List(c.listPath("gateways"), c.queryOptions(ctx))
	if err != nil {
		return nil, err
	}

	data := [][]byte{}
	for _, pair := range pairs {
		data = append(data, pair.Value)
	}
	return data, nil
}

func (c *ConsulBackend) DeleteGateway(ctx context.Context, id core.GatewayID) error {
	_, err := c.client.KV().Delete(c.storagePath("gateways", id.ConsulNamespace, id.Service), c.writeOptions(ctx))
	return err
}

func (c *ConsulBackend) UpsertGateways(ctx context.Context, gateways ...GatewayRecord) error {
	operations := consulapi.TxnOps{}

	for _, gateway := range gateways {
		operations = append(operations, &consulapi.TxnOp{
			KV: &consulapi.KVTxnOp{
				Verb:  consulapi.KVSet,
				Key:   c.storagePath("gateways", gateway.ID.ConsulNamespace, gateway.ID.Service),
				Value: gateway.Data,
			},
		})
	}

	_, _, _, err := c.client.Txn().Txn(operations, c.queryOptions(ctx))
	return err
}

func (c *ConsulBackend) GetRoute(ctx context.Context, id string) ([]byte, error) {
	pair, _, err := c.client.KV().Get(c.storagePath("routes", "default", id), c.queryOptions(ctx))
	if err != nil {
		return nil, err
	}
	if pair == nil {
		return nil, ErrNotFound
	}
	return pair.Value, nil
}

func (c *ConsulBackend) ListRoutes(ctx context.Context) ([][]byte, error) {
	pairs, _, err := c.client.KV().List(c.listPath("routes"), c.queryOptions(ctx))
	if err != nil {
		return nil, err
	}

	data := [][]byte{}
	for _, pair := range pairs {
		data = append(data, pair.Value)
	}
	return data, nil
}

func (c *ConsulBackend) DeleteRoute(ctx context.Context, id string) error {
	_, err := c.client.KV().Delete(c.storagePath("routes", "default", id), c.writeOptions(ctx))
	return err
}

func (c *ConsulBackend) UpsertRoutes(ctx context.Context, routes ...RouteRecord) error {
	operations := consulapi.TxnOps{}

	for _, route := range routes {
		operations = append(operations, &consulapi.TxnOp{
			KV: &consulapi.KVTxnOp{
				Verb:  consulapi.KVSet,
				Key:   c.storagePath("routes", "default", route.ID), // this should decompose a complex route ID in the future
				Value: route.Data,
			},
		})
	}

	_, _, _, err := c.client.Txn().Txn(operations, c.queryOptions(ctx))
	return err
}

// this should eventually be used to construct paths like:
//   v1/gateways/ns/default/foo-bar-baz
//   v1/http-routes/ns/default/foo-bar-baz
//   v1/tcp-routes/ns/default/foo-bar-baz
//
// TODO: for this to be fully functional we need to make sure that our route IDs encode this information
// via some compound identifier rather than by just a "string" as they do now
func (c *ConsulBackend) storagePath(entity, namespace, id string) string {
	return strings.Join([]string{c.pathPrefix, "v1", entity, "ns", namespace, id}, "/")
}

func (c *ConsulBackend) listPath(entity string) string {
	return strings.Join([]string{c.pathPrefix, "v1", entity}, "/")
}

func (c *ConsulBackend) queryOptions(ctx context.Context) *consulapi.QueryOptions {
	opts := &consulapi.QueryOptions{}
	if c.namespace != "" {
		opts.Namespace = c.namespace
	}
	return opts.WithContext(ctx)
}

func (c *ConsulBackend) writeOptions(ctx context.Context) *consulapi.WriteOptions {
	opts := &consulapi.WriteOptions{}
	if c.namespace != "" {
		opts.Namespace = c.namespace
	}
	return opts.WithContext(ctx)
}
