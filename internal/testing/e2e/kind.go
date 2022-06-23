package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var (
	kindTemplate       *template.Template
	kindTemplateString = `
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.23.6@sha256:b1fa224cc6c7ff32455e0b1fd9cbfd3d3bc87ecaa8fcb06961ed1afb3db0f9ae
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: {{ .HTTPSPort }}
    hostPort: {{ .HTTPSPort }}
    protocol: TCP
  - containerPort: {{ .HTTPSFlattenedPort }}
    hostPort: {{ .HTTPSFlattenedPort }}
    protocol: TCP
  - containerPort: {{ .HTTPSReferencePolicyPort }}
    hostPort: {{ .HTTPSReferencePolicyPort }}
    protocol: TCP
  - containerPort: {{ .TCPReferencePolicyPort }}
    hostPort: {{ .TCPReferencePolicyPort }}
    protocol: TCP
  - containerPort: {{ .ParentRefChangeFirstGatewayPort }}
    hostPort: {{ .ParentRefChangeFirstGatewayPort }}
    protocol: TCP
  - containerPort: {{ .ParentRefChangeSecondGatewayPort }}
    hostPort: {{ .ParentRefChangeSecondGatewayPort }}
    protocol: TCP
  - containerPort: {{ .GRPCPort }}
    hostPort: {{ .GRPCPort }}
    protocol: TCP
  - containerPort: {{ .ExtraTCPPort }}
    hostPort: {{ .ExtraTCPPort }}
    protocol: TCP
  - containerPort: {{ .ExtraHTTPPort }}
    hostPort: {{ .ExtraHTTPPort }}
    protocol: TCP
  - containerPort: {{ .ExtraTCPTLSPort }}
    hostPort: {{ .ExtraTCPTLSPort }}
    protocol: TCP
  - containerPort: {{ .ExtraTCPTLSPortTwo }}
    hostPort: {{ .ExtraTCPTLSPortTwo }}
    protocol: TCP
`
)

type kindContextKey string

func init() {
	kindTemplate = template.Must(template.New("kind").Parse(kindTemplateString))
}

// based off github.com/kubernetes-sigs/e2e-framework/support/kind
type kindCluster struct {
	name                             string
	e                                *gexe.Echo
	kubecfgFile                      string
	config                           string
	httpsPort                        int
	httpsFlattenedPort               int
	httpsReferencePolicyPort         int
	tcpReferencePolicyPort           int
	parentRefChangeFirstGatewayPort  int
	parentRefChangeSecondGatewayPort int
	grpcPort                         int
	extraHTTPPort                    int
	extraTCPPort                     int
	extraTCPTLSPort                  int
	extraTCPTLSPortTwo               int
}

func newKindCluster(name string) *kindCluster {
	ports := freeport.MustTake(11)
	return &kindCluster{
		name:                             name,
		e:                                gexe.New(),
		httpsPort:                        ports[0],
		httpsFlattenedPort:               ports[1],
		httpsReferencePolicyPort:         ports[2],
		tcpReferencePolicyPort:           ports[3],
		parentRefChangeFirstGatewayPort:  ports[4],
		parentRefChangeSecondGatewayPort: ports[5],
		grpcPort:                         ports[6],
		extraHTTPPort:                    ports[7],
		extraTCPPort:                     ports[8],
		extraTCPTLSPort:                  ports[9],
		extraTCPTLSPortTwo:               ports[10],
	}
}

func (k *kindCluster) Create() (string, error) {
	log.Println("Creating kind cluster", k.name)

	var kindConfig bytes.Buffer
	err := kindTemplate.Execute(&kindConfig, &struct {
		HTTPSPort                        int
		HTTPSFlattenedPort               int
		HTTPSReferencePolicyPort         int
		TCPReferencePolicyPort           int
		ParentRefChangeFirstGatewayPort  int
		ParentRefChangeSecondGatewayPort int
		GRPCPort                         int
		ExtraTCPPort                     int
		ExtraTCPTLSPort                  int
		ExtraTCPTLSPortTwo               int
		ExtraHTTPPort                    int
	}{
		HTTPSPort:                        k.httpsPort,
		HTTPSFlattenedPort:               k.httpsFlattenedPort,
		HTTPSReferencePolicyPort:         k.httpsReferencePolicyPort,
		TCPReferencePolicyPort:           k.tcpReferencePolicyPort,
		ParentRefChangeFirstGatewayPort:  k.parentRefChangeFirstGatewayPort,
		ParentRefChangeSecondGatewayPort: k.parentRefChangeSecondGatewayPort,
		GRPCPort:                         k.grpcPort,
		ExtraTCPPort:                     k.extraTCPPort,
		ExtraTCPTLSPort:                  k.extraTCPTLSPort,
		ExtraTCPTLSPortTwo:               k.extraTCPTLSPortTwo,
		ExtraHTTPPort:                    k.extraHTTPPort,
	})
	if err != nil {
		return "", err
	}

	if strings.Contains(k.e.Run("kind get clusters"), k.name) {
		log.Println("Skipping Kind Cluster.Create: cluster already created: ", k.name)
		return "", nil
	}

	config, err := ioutil.TempFile("", "kind-cluster-config")
	if err != nil {
		return "", nil
	}
	defer config.Close()

	k.config = config.Name()

	if n, err := io.Copy(config, &kindConfig); n == 0 || err != nil {
		return "", fmt.Errorf("kind kubecfg file: bytes copied: %d: %w]", n, err)
	}

	// create kind cluster using kind-cluster-docker.yaml config file
	log.Println("launching: kind create cluster --name", k.name)
	p := k.e.RunProc(fmt.Sprintf(`kind create cluster --name %s --config %s`, k.name, config.Name()))
	if p.Err() != nil {
		return "", fmt.Errorf("failed to create kind cluster: %s : %s", p.Err(), p.Result())
	}

	// grab kubeconfig file for cluster
	kubecfg := fmt.Sprintf("%s-kubecfg", k.name)
	p = k.e.StartProc(fmt.Sprintf(`kind get kubeconfig --name %s`, k.name))
	if p.Err() != nil {
		return "", fmt.Errorf("kind get kubeconfig: %s: %w", p.Result(), p.Err())
	}

	file, err := ioutil.TempFile("", fmt.Sprintf("kind-cluser-%s", kubecfg))
	if err != nil {
		return "", fmt.Errorf("kind kubeconfig file: %w", err)
	}
	defer file.Close()

	k.kubecfgFile = file.Name()

	if n, err := io.Copy(file, p.Out()); n == 0 || err != nil {
		return "", fmt.Errorf("kind kubecfg file: bytes copied: %d: %w]", n, err)
	}

	return file.Name(), nil
}

func (k *kindCluster) Destroy() error {
	log.Println("Destroying kind cluster ", k.name)

	// deleteting kind cluster
	p := k.e.RunProc(fmt.Sprintf(`kind delete cluster --name %s`, k.name))
	if p.Err() != nil {
		return fmt.Errorf("failed to install kind: %s: %s", p.Err(), p.Result())
	}

	log.Println("Removing kubeconfig file ", k.kubecfgFile)
	if err := os.RemoveAll(k.kubecfgFile); err != nil {
		return fmt.Errorf("kind: remove kubefconfig failed: %w", err)
	}

	log.Println("Removing config file ", k.config)
	if err := os.RemoveAll(k.config); err != nil {
		return fmt.Errorf("kind: remove config failed: %w", err)
	}

	return nil
}

// https://github.com/kubernetes-sigs/e2e-framework/blob/63fa8b05c52cc136a3e529b9f9f812b061cea165/pkg/envfuncs/kind_funcs.go#L38
func CreateKindCluster(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		k := newKindCluster(clusterName)
		kubecfg, err := k.Create()
		if err != nil {
			return ctx, err
		}

		// stall, wait for pods initializations
		time.Sleep(7 * time.Second)

		// update envconfig  with kubeconfig
		if _, err := cfg.WithKubeconfigFile(kubecfg); err != nil {
			return ctx, fmt.Errorf("create kind cluster func: update envconfig: %w", err)
		}

		// store entire cluster value in ctx for future access using the cluster name
		return context.WithValue(ctx, kindContextKey(clusterName), k), nil
	}
}

// https://github.com/kubernetes-sigs/e2e-framework/blob/63fa8b05c52cc136a3e529b9f9f812b061cea165/pkg/envfuncs/kind_funcs.go#L64
func DestroyKindCluster(name string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		clusterVal := ctx.Value(kindContextKey(name))
		if clusterVal == nil {
			return ctx, fmt.Errorf("destroy kind cluster func: context cluster is nil")
		}

		cluster, ok := clusterVal.(*kindCluster)
		if !ok {
			return ctx, fmt.Errorf("destroy kind cluster func: unexpected type for cluster value")
		}

		if err := cluster.Destroy(); err != nil {
			return ctx, fmt.Errorf("destroy kind cluster: %w", err)
		}

		return ctx, nil
	}
}

func LoadKindDockerImage(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Println("Loading docker image into kind cluster")

		if err := loadImage(ctx, clusterName, DockerImage(ctx)); err != nil {
			return nil, err
		}

		for _, image := range ExtraDockerImages() {
			log.Printf("Loading additional docker image:%s into kind cluster", image)
			if err := loadImage(ctx, clusterName, image); err != nil {
				return nil, err
			}
		}

		return ctx, nil
	}
}

func loadImage(ctx context.Context, clusterName, image string) error {
	var stdout, stderr bytes.Buffer
	timeoutContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutContext, "kind", "load", "docker-image", image, image, "--name", clusterName)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return errors.New(stderr.String())
	}
	return nil
}
