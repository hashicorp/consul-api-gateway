// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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

	"github.com/vladimirvivien/gexe"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/hashicorp/consul/sdk/freeport"
)

var (
	kindTemplate       *template.Template
	kindTemplateString = `
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.24.6@sha256:97e8d00bc37a7598a0b32d1fabd155a96355c49fa0d4d4790aab0f161bf31be1
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
  - containerPort: {{ .HTTPSReferenceGrantPort }}
    hostPort: {{ .HTTPSReferenceGrantPort }}
    protocol: TCP
  - containerPort: {{ .TCPReferenceGrantPort }}
    hostPort: {{ .TCPReferenceGrantPort }}
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
	httpsReferenceGrantPort          int
	tcpReferenceGrantPort            int
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
		httpsReferenceGrantPort:          ports[2],
		tcpReferenceGrantPort:            ports[3],
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
		HTTPSReferenceGrantPort          int
		TCPReferenceGrantPort            int
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
		HTTPSReferenceGrantPort:          k.httpsReferenceGrantPort,
		TCPReferenceGrantPort:            k.tcpReferenceGrantPort,
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

// https://github.com/kubernetes-sigs/e2e-framework/blob/2aa1046b47656cde5c9ed2d6a0c58a86e70b43eb/pkg/envfuncs/kind_funcs.go#L43
func CreateKindCluster(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// use custom cluster creation func to reserve ports
		k := newKindCluster(clusterName)
		kubecfg, err := k.Create()
		if err != nil {
			return ctx, err
		}

		// update envconfig  with kubeconfig
		cfg.WithKubeconfigFile(kubecfg)

		// stall, wait for pods initializations
		if err := waitForControlPlane(cfg.Client()); err != nil {
			return ctx, err
		}

		// store entire cluster value in ctx for future access using the cluster name
		return context.WithValue(ctx, kindContextKey(clusterName), k), nil
	}
}

// https://github.com/kubernetes-sigs/e2e-framework/blob/2aa1046b47656cde5c9ed2d6a0c58a86e70b43eb/pkg/envfuncs/kind_funcs.go#L71
func waitForControlPlane(client klient.Client) error {
	r, err := resources.New(client.RESTConfig())
	if err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "component", Operator: metav1.LabelSelectorOpIn, Values: []string{"etcd", "kube-apiserver", "kube-controller-manager", "kube-scheduler"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 pods running the core apiserver components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	selector, err = metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "k8s-app", Operator: metav1.LabelSelectorOpIn, Values: []string{"kindnet", "kube-dns", "kube-proxy"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 k8s-app pods running networking components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	return nil
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
