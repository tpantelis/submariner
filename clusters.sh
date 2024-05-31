#!/usr/bin/env bash

source "${SCRIPTS_DIR}"/lib/lusters_kind

kind_k8s_versions[1.28]=1.28.0@sha256:b7a4cad12c197af3ba43202d3efe03246b3f0793f162afb40a33c923952d5b31

"${SCRIPTS_DIR}"/clusters.sh "$@"
