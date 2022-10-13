package exec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul-api-gateway/internal/consul"
	"github.com/hashicorp/consul-api-gateway/internal/envoy"
)

type AuthConfig struct {
	Method    string
	Namespace string
	Token     string
}

type GatewayConfig struct {
	Host      string
	Name      string
	Namespace string
}

type EnvoyConfig struct {
	CACertificateFile    string
	XDSAddress           string
	XDSPort              int
	SDSAddress           string
	SDSPort              int
	BootstrapFile        string
	CertificateDirectory string
	Binary               string
	ExtraArgs            []string
	Output               io.Writer
}

type ExecConfig struct {
	Context           context.Context
	Logger            hclog.Logger
	LogLevel          string
	ConsulClient      *api.Client
	ConsulConfig      api.Config
	ConsulHTTPAddress string
	ConsulHTTPPort    int
	ConsulGRPCPort    int
	AuthConfig        AuthConfig
	GatewayConfig     GatewayConfig
	EnvoyConfig       EnvoyConfig
	PrimaryDatacenter string

	// for testing only
	isTest bool
}

func RunExec(config ExecConfig) (ret int) {
	// Set up signal handlers and global context
	ctx, cancel := context.WithCancel(config.Context)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// ConsulServerConnMgr is the watcher for the Consul server addresses.
	consulServerConnMgr, err := discovery.NewWatcher(ctx, discovery.Config{
		Addresses: config.ConsulHTTPAddress,
		GRPCPort:  config.ConsulGRPCPort,
		// TLS:       config.ConsulConfig.TLSConfig,
		Credentials: discovery.Credentials{
			Type: discovery.CredentialsTypeLogin,
			Login: discovery.LoginCredential{
				AuthMethod: config.AuthConfig.Method,
				Namespace:  config.AuthConfig.Method,
				// Partition:   "",
				// Datacenter:  "",
				BearerToken: config.AuthConfig.Token,
			},
		},
	}, config.Logger)
	if err != nil {
		config.Logger.Error("failed to start Consul server connection manager", err)
		return 1
	}

	// Start Consul server discovery connection manager watcher
	go consulServerConnMgr.Run()
	defer consulServerConnMgr.Stop()

	// Configure signal handlers to shut down cleanly
	defer func() {
		signal.Stop(interrupt)
		consulServerConnMgr.Stop()
		cancel()
	}()
	go func() {
		select {
		case <-interrupt:
			config.Logger.Debug("received shutdown signal")
			consulServerConnMgr.Stop()
			cancel()
		case <-ctx.Done():
			consulServerConnMgr.Stop()
		}
	}()

	// Wait for initial state.
	serverState, err := consulServerConnMgr.State()
	if err != nil {
		config.Logger.Error("failed to get Consul server state", err)
		return 1
	}
	config.Logger.Trace("%#v", serverState)

	// First do the ACL Login, if necessary, and create a client the first time.
	var consulClient *api.Client
	var token string

	if config.AuthConfig.Method != "" {
		config.Logger.Trace("logging in to consul")
		consulClient, token, err = login(config, serverState)
		if err != nil {
			config.Logger.Error("error logging into Consul", "error", err)
			return 1
		}
		config.Logger.Trace("consul login complete")
	} else {
		config.ConsulConfig.Namespace = config.GatewayConfig.Namespace
		consulClient, err = makeClient(config, serverState)
		if err != nil {
			config.Logger.Error("error initializing Consul client", "error", err)
			return 1
		}
	}

	registry := consul.NewServiceRegistry(
		config.Logger.Named("service-registry"),
		consulClient,
		config.GatewayConfig.Name,
		config.GatewayConfig.Namespace,
		config.GatewayConfig.Host,
	)
	if config.isTest {
		registry = registry.WithTries(1)
	}

	// Subscribe to the Watcher. This returns a channel that receives
	// a new discovery.State whenever the Watches connects to another
	// Consul server
	ch := consulServerConnMgr.Subscribe()

	// Monitor the channel and rebuild the client when needed
	for {
		select {
		case state := <-ch:
			consulClient, err := makeClient(config, state)

			if err != nil {
				config.Logger.Error("failed to initialize Consul client from new Consul server discovery state", err)
				// TODO: should this exit in this case, or do something else like retry?
				return 1
			}

			registry.WithClient(consulClient)
		case <-ctx.Done():
		}
	}

	config.Logger.Trace("registering service")
	if err := registry.RegisterGateway(ctx, true); err != nil {
		config.Logger.Error("error registering service", "error", err)
		return 1
	}
	defer func() {
		config.Logger.Trace("deregistering service")
		// using context.Background here since the global context has
		// already been canceled at this point and we're just in a cleanup
		// function
		if err := registry.Deregister(context.Background()); err != nil {
			config.Logger.Error("error deregistering service", "error", err)
			ret = 1
		}
		// clean up the ACL token that was provisioned via acl.login
		if config.AuthConfig.Method != "" {
			if err := registry.Logout(context.Background(), token); err != nil {
				config.Logger.Error("error deleting acl token", "error", err)
				ret = 1
			}
		}
	}()

	envoyManager := envoy.NewManager(
		config.Logger.Named("envoy-manager"),
		envoy.ManagerConfig{
			ID:                registry.ID(),
			Namespace:         registry.Namespace(),
			ConsulCA:          config.EnvoyConfig.CACertificateFile,
			ConsulAddress:     config.EnvoyConfig.XDSAddress,
			ConsulXDSPort:     config.EnvoyConfig.XDSPort,
			BootstrapFilePath: config.EnvoyConfig.BootstrapFile,
			LogLevel:          config.LogLevel,
			Token:             token,
			EnvoyBinary:       config.EnvoyConfig.Binary,
			ExtraArgs:         config.EnvoyConfig.ExtraArgs,
			Output:            config.EnvoyConfig.Output,
		},
	)
	options := consul.DefaultCertManagerOptions()
	options.PrimaryDatacenter = config.PrimaryDatacenter
	options.SDSAddress = config.EnvoyConfig.SDSAddress
	options.SDSPort = config.EnvoyConfig.SDSPort
	options.Directory = "/certs"
	if config.EnvoyConfig.CertificateDirectory != "" {
		options.Directory = config.EnvoyConfig.CertificateDirectory
	}
	certManager := consul.NewCertManager(
		config.Logger.Named("cert-manager"),
		consulClient,
		config.GatewayConfig.Name,
		options,
	)
	sdsConfig, err := certManager.RenderSDSConfig()
	if err != nil {
		config.Logger.Error("error rendering SDS configuration files", "error", err)
		return 1
	}
	err = envoyManager.RenderBootstrap(sdsConfig)
	if err != nil {
		config.Logger.Error("error rendering Envoy configuration file", "error", err)
		return 1
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return certManager.Manage(groupCtx)
	})

	// wait until we've written once before booting envoy
	waitTime := defaultCertWaitTime
	if config.isTest {
		waitTime = 100 * time.Millisecond
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, waitTime)
	defer waitCancel()
	config.Logger.Trace("waiting for initial certs to be written")
	if err := certManager.WaitForWrite(waitCtx); err != nil {
		config.Logger.Error("timeout waiting for certs to be written", "error", err)
		return 1
	}
	config.Logger.Trace("initial certificates written")

	group.Go(func() error {
		return envoyManager.Run(ctx)
	})

	config.Logger.Info("started consul-api-gateway api gateway")
	if err := group.Wait(); err != nil {
		config.Logger.Error("unexpected error", "error", err)
		return 1
	}

	config.Logger.Info("shutting down")
	return 0
}

// The state contains the server address and ACL token (if applicable).
func makeClient(config ExecConfig, s discovery.State) (*api.Client, error) {
	config.ConsulConfig.Address = fmt.Sprintf("%s:%d", s.Address.IP.String(), config.ConsulHTTPPort)
	// config.ConsulConfig.Address = fmt.Sprintf("%s:%d", s.Address, config.ConsulHTTPPort)
	if s.Token != "" {
		config.ConsulConfig.Token = s.Token
	}
	// config.ConsulConfig.Token = s.Token

	// TODO: is this necessary?
	if config.ConsulConfig.Transport == nil {
		tlsClientConfig, err := api.SetupTLSConfig(&config.ConsulConfig.TLSConfig)

		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS transport: %w", err)
		}

		config.ConsulConfig.Transport = &http.Transport{TLSClientConfig: tlsClientConfig}
	} else if config.ConsulConfig.Transport.TLSClientConfig == nil {
		tlsClientConfig, err := api.SetupTLSConfig(&config.ConsulConfig.TLSConfig)

		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS transport: %w", err)
		}

		config.ConsulConfig.Transport.TLSClientConfig = tlsClientConfig
	}
	config.ConsulConfig.HttpClient.Transport = config.ConsulConfig.Transport

	return api.NewClient(&config.ConsulConfig)
}

func login(config ExecConfig, s discovery.State) (*api.Client, string, error) {
	consulClient, err := makeClient(config, s)
	if err != nil {
		return nil, "", fmt.Errorf("error creating consul client: %w", err)
	}

	authenticator := consul.NewAuthenticator(
		config.Logger.Named("authenticator"),
		consulClient,
		config.AuthConfig.Method,
		config.AuthConfig.Namespace,
	)
	if config.isTest {
		authenticator = authenticator.WithTries(1)
	}

	token, err := authenticator.Authenticate(config.Context, config.GatewayConfig.Name, config.AuthConfig.Token)
	if err != nil {
		return nil, "", fmt.Errorf("error logging in to consul: %w", err)
	}

	// Now update the client so that it will read the ACL token we just fetched.
	config.ConsulConfig.Token = token
	config.ConsulConfig.Namespace = config.GatewayConfig.Namespace
	newClient, err := makeClient(config, s)
	if err != nil {
		return nil, "", fmt.Errorf("error updating client connection with token: %w", err)
	}
	return newClient, token, nil
}
