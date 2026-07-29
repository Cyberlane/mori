#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s <v-semver>\n' "$0" >&2
  exit 2
fi

numeric='(0|[1-9][0-9]*)'
prerelease_identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
build_identifier='[0-9A-Za-z-]+'
pattern="^v${numeric}\\.${numeric}\\.${numeric}(-${prerelease_identifier}(\\.${prerelease_identifier})*)?(\\+${build_identifier}(\\.${build_identifier})*)?$"

if [[ ! $1 =~ $pattern ]]; then
  printf 'invalid release tag: %s\n' "$1" >&2
  printf 'expected a v-prefixed strict Semantic Versioning tag\n' >&2
  exit 1
fi
