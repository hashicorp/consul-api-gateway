package sdsserver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/hashicorp/polar/sds"
)

func (c *cmd) loadStaticSecrets(s *sds.Server) error {
	bs, err := ioutil.ReadFile(c.secretFile)
	if err != nil {
		return err
	}

	var secrets map[string]secret

	if err := json.Unmarshal(bs, &secrets); err != nil {
		return err
	}

	for name, secret := range secrets {
		c.UI.Info(fmt.Sprintf("Loading secret `%s` from file", name))
		s.UpdateTLSSecret(name, []byte(secret.Cert), []byte(secret.Key))
	}
	return nil
}

type secret struct {
	Key  string
	Cert string
}
