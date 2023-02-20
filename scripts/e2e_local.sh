#!/bin/bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


set -eEuo pipefail

run_test() {
    E2E_APIGW_CONSUL_IMAGE="$E2E_APIGW_CONSUL_IMAGE" DOCKER_HOST_ROUTE="$DOCKER_HOST_ROUTE" go test -short -v -failfast -tags e2e ./internal/commands/server
}

check_env_vars() {
    # if enterprise image check that the license env var is set
    if [[ "$E2E_APIGW_CONSUL_IMAGE" == *"ent"* && "$CONSUL_LICENSE" == "" ]]; then
        echo "You are running the e2e tests against enterprise consul without an enterprise license env var set. Set an env var named \"CONSUL_LICENSE\" to a valid license and run again"
        exit 1
    fi

    # if running on linux the DOCKER_HOST_ROUTE should be set to the docker IP address
    if [[ "$(uname -s)" == "Linux" ]]; then
        DOCKER_HOST_ROUTE="172.17.0.1"
    fi
}

main() {
    check_env_vars
    run_test
}

main
