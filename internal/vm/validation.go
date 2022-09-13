package vm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/hashicorp/consul-api-gateway/internal/api/v1"
	"github.com/hashicorp/consul-api-gateway/internal/common"
	"github.com/hashicorp/consul-api-gateway/internal/vault"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
)

// Validator is responsible for validating the more complex fields of Gateways and Routes
type Validator struct {
	client      *consulapi.Client
	vaultClient vault.KVClient
	logger      hclog.Logger
}

func NewValidator(logger hclog.Logger, vaultClient vault.KVClient, client *consulapi.Client) *Validator {
	return &Validator{
		client:      client,
		vaultClient: vaultClient,
		logger:      logger,
	}
}

// ValidateGateway validates a Gateway definition
func (v *Validator) ValidateGateway(ctx context.Context, gateway *v1.Gateway) error {
	var errs multierror.Error
	listenerNames := map[string]int{}
	listenerPorts := map[int]int{}

	for i, listener := range gateway.Listeners {
		// treat unsupplied name as a default name (empty string)
		name := ""
		if listener.Name != nil {
			name = *listener.Name
		}

		// make sure that each listener has a unique port and name
		if index, exists := listenerNames[name]; exists {
			errs.Errors = append(errs.Errors, fmt.Errorf("listener.%d: name %q conflicts with listener.%d", i, name, index))
		} else {
			listenerNames[name] = i
		}

		if index, exists := listenerPorts[listener.Port]; exists {
			errs.Errors = append(errs.Errors, fmt.Errorf("listener.%d: port %d conflicts with listener.%d", i, listener.Port, index))
		} else {
			listenerPorts[listener.Port] = i
		}

		// validate TLS
		errs.Errors = append(errs.Errors, v.validateListerTLS(i, listener.Tls)...)

		// validate TLS certs
		errs.Errors = append(errs.Errors, v.validateListerTLSCertificates(ctx, i, listener.Tls)...)
	}

	return errs.ErrorOrNil()
}

func (v *Validator) validateListerTLS(index int, tls *v1.TLSConfiguration) []error {
	if tls == nil {
		return nil
	}

	minVersion := tls.MinVersion
	maxVersion := tls.MaxVersion
	cipherSuites := tls.CipherSuites
	errs := []error{}

	if minVersion != nil {
		if _, ok := common.SupportedTLSVersions[*minVersion]; !ok {
			errs = append(errs, fmt.Errorf("listener.%d.tls.minVersion: invalid TLS version %q", index, *minVersion))
		} else {
			if len(cipherSuites) > 0 {
				if _, ok := common.TLSVersionsWithConfigurableCipherSuites[*minVersion]; !ok {
					errs = append(errs, fmt.Errorf("listener.%d.tls.cipherSuites: configuring TLS cipher suites is only supported for TLS 1.2 and earlier", index))
				}
			}
		}
	}

	if maxVersion != nil {
		if _, ok := common.SupportedTLSVersions[*maxVersion]; !ok {
			errs = append(errs, fmt.Errorf("listener.%d.tls.maxVersion: invalid TLS version %q", index, *maxVersion))
		}
	}

	for i, c := range cipherSuites {
		if ok := common.SupportedTLSCipherSuite(c); !ok {
			errs = append(errs, fmt.Errorf("listener.%d.tls.cipherSuites.%d: unsupported TLS cipher suite %q", index, i, c))
		}
	}

	return errs
}

func (v *Validator) validateListerTLSCertificates(ctx context.Context, index int, tls *v1.TLSConfiguration) []error {
	if tls == nil {
		return nil
	}

	errs := []error{}

	if len(tls.Certificates) == 0 {
		errs = append(errs, fmt.Errorf("listener.%d.tls.certificates: certificates must be specified if TLS is enabled", index))
	} else {
		for i, certificate := range tls.Certificates {
			if err := v.validateCertificate(ctx, certificate); err != nil {
				errs = append(errs, fmt.Errorf("listener.%d.tls.certificates.%d: %s", index, i, err))
			}
		}
	}

	return errs
}

func (v *Validator) validateCertificate(ctx context.Context, cert v1.Certificate) error {
	// make sure that only one certificate field is specified
	setFields := checkMultiFieldSet(map[string]interface{}{
		"vault": cert.Vault,
	})
	if len(setFields) == 0 {
		return errors.New("one certificate field must be set")
	}
	if len(setFields) > 1 {
		return fmt.Errorf("must specify only one of %q", strings.Join(setFields, ", "))
	}

	if cert.Vault != nil {
		return v.validateVaultKVCertificate(ctx, cert.Vault)
	}

	return nil
}

func (v *Validator) validateVaultKVCertificate(ctx context.Context, cert *v1.VaultCertificate) error {
	secret, err := v.vaultClient.Get(ctx, cert.Path)
	if err != nil {
		// log the error and return something more generic
		v.logger.Error("validating vault certificate", "error", err)
		return errors.New("unable to retrieve Vault certificate")
	}

	if _, ok := secret.Data[cert.ChainField]; !ok {
		return fmt.Errorf("invalid Vault field %q for certificate chain", cert.ChainField)
	}

	if _, ok := secret.Data[cert.PrivateKeyField]; !ok {
		return fmt.Errorf("invalid Vault field %q for certificate private key", cert.PrivateKeyField)
	}

	return nil
}

// ValidateHTTPRoute validates an HTTPRoute definition
func (v *Validator) ValidateHTTPRoute(ctx context.Context, route *v1.HTTPRoute) error {
	var errs multierror.Error
	for _, rule := range route.Rules {
		for _, service := range rule.Services {
			// check for the existence of the upstream service
			if err := v.checkService(ctx, service.Name, service.Namespace); err != nil {
				errs.Errors = append(errs.Errors, err)
			}
		}
	}
	return errs.ErrorOrNil()
}

// ValidateTCPRoute validates an TCPRoute definition
func (v *Validator) ValidateTCPRoute(ctx context.Context, route *v1.TCPRoute) error {
	var errs multierror.Error
	for _, service := range route.Services {
		// check for the existence of the upstream service
		if err := v.checkService(ctx, service.Name, service.Namespace); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	return errs.ErrorOrNil()
}

func (v *Validator) checkService(ctx context.Context, name string, namespace *string) error {
	opts := &consulapi.QueryOptions{}
	if namespace != nil {
		opts.Namespace = *namespace
	}
	instances, _, err := v.client.Catalog().Service(name, "", opts.WithContext(ctx))
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return fmt.Errorf("no service %q found", name)
	}

	connectEnabled := false
	for _, instance := range instances {
		// TODO: is there a better way of ensuring that this is Connect enabled?
		if instance.ServiceProxy != nil && instance.ServiceProxy.DestinationServiceName != "" {
			connectEnabled = true
			break
		}
	}
	if !connectEnabled {
		return fmt.Errorf("service %q is not connect enabled", name)
	}

	return nil
}

func checkMultiFieldSet(fields map[string]interface{}) []string {
	isSet := []string{}
	for k, v := range fields {
		if v != nil {
			isSet = append(isSet, k)
		}
	}
	return isSet
}
