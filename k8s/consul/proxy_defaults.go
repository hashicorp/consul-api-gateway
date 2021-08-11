package consul

import (
	"bytes"
	"html/template"

	"github.com/hashicorp/consul/api"
)

var (
	defaultSDSClusterJSONArgs = sdsClusterArgs{
		Name:              "sds-cluster",
		CertSDSConfigPath: "/etc/envoy/tls_sds.yaml",
		CASDSConfigPath:   "/etc/envoy/validation_context_sds.yaml",
		SDSAddress:        "127.0.0.1",
		SDSPort:           9090,
	}
	sdsClusterTemplate = template.New("sdsCluster")
)

func init() {
	_, err := sdsClusterTemplate.Parse(sdsClusterJSONTemplate)
	if err != nil {
		panic(err)
	}
}

func proxyDefaults(meta map[string]string, args sdsClusterArgs) (*api.ProxyConfigEntry, error) {
	clusterJSON, err := generateSDSClusterJSON(args)
	if err != nil {
		return nil, err
	}
	return &api.ProxyConfigEntry{
		Kind: api.ProxyDefaults,
		Name: "global",
		//Namespace: namespace, // TODO
		Config: map[string]interface{}{
			"envoy_extra_static_clusters_json": clusterJSON,
		},
		Meta: meta,
	}, nil
}

type sdsClusterArgs struct {
	Name              string
	CertSDSConfigPath string
	CASDSConfigPath   string
	SDSAddress        string
	SDSPort           int
}

func generateSDSClusterJSON(args sdsClusterArgs) (string, error) {
	var buf bytes.Buffer
	if err := sdsClusterTemplate.Execute(&buf, args); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const sdsClusterJSONTemplate = `{
   "name":"{{ .Name }}",
   "connect_timeout":"5s",
   "type":"STATIC",
   "transport_socket":{
      "name":"tls",
      "typed_config":{
         "@type":"type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext",
         "common_tls_context":{
            "tls_certificate_sds_secret_configs":[
               {
                  "name":"tls_sds",
                  "sds_config":{
                     "path":"{{ .CertSDSConfigPath }}"
                  }
               }
            ],
            "validation_context_sds_secret_config":{
               "name":"validation_context_sds",
               "sds_config":{
                  "path":"{{ .CASDSConfigPath }}"
               }
            }
         }
      }
   },
   "http2_protocol_options":{
      
   },
   "loadAssignment":{
      "clusterName":"{{ .Name }}",
      "endpoints":[
         {
            "lbEndpoints":[
               {
                  "endpoint":{
                     "address":{
                        "socket_address":{
                           "address":"{{ .SDSAddress }}",
                           "port_value":{{ .SDSPort }}
                        }
                     }
                  }
               }
            ]
         }
      ]
   }
}
`
