package v1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/consul/api"
)

const consulTokenHeader = "X-Consul-Token"
const globalManagementID = "00000000-0000-0000-0000-000000000001"

func (s *Server) consulTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(consulTokenHeader)
		acl, _, err := s.consulClient.ACL().TokenReadSelf(&api.QueryOptions{Token: token})

		fmt.Println("acl", acl)
		fmt.Println("token", token)
		//Id acl is disabled, don't serve unauthenticated error
		if err != nil && aclIsDisabled(err) {
			next.ServeHTTP(w, r)
			return
		}

		if !hasGlobalManagementToken(acl) {
			s.sendError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if err != nil {

			s.logger.Error("checking token acl", "error", err)
			s.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		next.ServeHTTP(w, r)
	})
}

func aclIsDisabled(err error) bool {
	var remoteErr api.StatusError
	if errors.As(err, &remoteErr) {
		return remoteErr.Code == http.StatusUnauthorized && remoteErr.Body == "ACL support disabled"
	}
	return false
}

func hasGlobalManagementToken(acl *api.ACLToken) bool {
	if acl == nil {
		return false
	}
	for _, policy := range acl.Policies {
		if policy.ID == globalManagementID {
			return true
		}
	}
	return false
}
