package consul

import (
	"github.com/hashicorp/consul/api"
)

type CertGenerator struct {
	consul *api.Client
}

type Cert struct {
	Root   *api.CARoot
	Client *api.LeafCert
}

func NewCertGenerator(consul *api.Client) *CertGenerator {
	return &CertGenerator{
		consul: consul,
	}
}

func (c *CertGenerator) GenerateFor(service string) (*Cert, error) {
	clientCert, _, err := c.consul.Agent().ConnectCALeaf(service, nil)
	if err != nil {
		return nil, err
	}

	cert := &Cert{
		Client: clientCert,
	}

	roots, _, err := c.consul.Agent().ConnectCARoots(nil)
	if err != nil {
		return nil, err
	}
	for _, root := range roots.Roots {
		if root.Active {
			cert.Root = root
			break
		}
	}

	return cert, nil
}
