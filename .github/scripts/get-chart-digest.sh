#!/bin/bash
# Get digest of published Helm chart from OCI registry
set -euo pipefail

OCI_REF="$1"

digest=$(crane digest "$OCI_REF")
oci_name="${OCI_REF%%:*}"

{
  echo "oci_name=${oci_name}"
  echo "digest=${digest}"
} >> "$GITHUB_OUTPUT"

echo "Resolved $OCI_REF → $digest"
