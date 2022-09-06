package v1

import (
	"errors"
	"net/http"

	"github.com/hashicorp/consul/api"
)

const (
	consulTokenHeader  = "X-Consul-Token"
	invalidToken       = "invalid-token"
	globalManagementID = "00000000-0000-0000-0000-000000000001"
)

func (s *Server) consulTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(consulTokenHeader)
		if token == "" {
			// we need to make sure that token is set in order to not leverage
			// the underlying token defaulted on the client
			token = invalidToken
		}
		acl, _, err := s.consulClient.ACL().TokenReadSelf(&api.QueryOptions{Token: token})

		//if acls are disabled, don't serve unauthenticated error
		if err != nil && aclIsDisabled(err) {
			next.ServeHTTP(w, r)
			return
		}

		if !hasGlobalManagementToken(acl) {
			if err != nil && !aclNotFound(err) {
				s.logger.Error("checking token acl", "error", err)
				sendError(w, http.StatusInternalServerError, err.Error())
				return
			}
			sendError(w, http.StatusUnauthorized, "unauthorized")
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

func aclNotFound(err error) bool {
	var remoteErr api.StatusError
	if errors.As(err, &remoteErr) {
		return remoteErr.Code == http.StatusForbidden && remoteErr.Body == "ACL not found"
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
