package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		namespace    string
		service      string
		expectedMeta string
		failures     uint64
		maxAttempts  uint64
		fail         bool
	}{{
		name:         "success-no-namespace",
		service:      "polar-test",
		expectedMeta: "polar-test",
	}, {
		name:         "success-namespace",
		namespace:    "foo",
		service:      "polar-test",
		expectedMeta: "foo/polar-test",
	}, {
		name:         "retry-success",
		service:      "polar-test",
		expectedMeta: "polar-test",
		failures:     3,
		maxAttempts:  3,
	}, {
		name:        "retry-failure",
		service:     "polar-test",
		failures:    3,
		maxAttempts: 2,
		fail:        true,
	}} {
		t.Run(test.name, func(t *testing.T) {
			server := runACLServer(t, test.failures)
			method := randomString()
			token := randomString()

			maxAttempts := defaultMaxAttempts
			if test.maxAttempts > 0 {
				maxAttempts = test.maxAttempts
			}

			auth := NewAuthenticator(hclog.NewNullLogger(), server.consul, method, test.namespace).WithTries(maxAttempts)
			auth.backoffInterval = 0
			consulToken, err := auth.Authenticate(context.Background(), test.service, token)

			if test.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, method, server.lastRequest.AuthMethod)
			require.Equal(t, token, server.lastRequest.BearerToken)
			require.Equal(t, test.expectedMeta, server.lastRequest.Meta[authMetaKey])
			require.Equal(t, server.consulToken, consulToken)
		})
	}
}

type aclServer struct {
	consul *api.Client

	consulToken string
	lastRequest api.ACLLoginParams
}

func runACLServer(t *testing.T, failures uint64) *aclServer {
	t.Helper()

	server := &aclServer{
		consulToken: randomString(),
	}

	// Start the fake Consul server.
	consulServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != nil && r.URL.Path == "/v1/acl/login" && r.Method == "POST" {
			if failures > 0 {
				failures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Errorf("error reading request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if err := json.Unmarshal(body, &server.lastRequest); err != nil {
				t.Errorf("error unmarshaling request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(fmt.Sprintf(`{"SecretID": "%s"}`, server.consulToken)))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consulServer.Close)

	serverURL, err := url.Parse(consulServer.URL)
	require.NoError(t, err)
	clientConfig := &api.Config{Address: serverURL.String()}
	client, err := api.NewClient(clientConfig)
	require.NoError(t, err)

	server.consul = client
	return server
}
