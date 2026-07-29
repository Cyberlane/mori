#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

for subject in \
  'feat: add parser' \
  'fix(parser): reject broken tree' \
  'chore(ci): update runner' \
  'style: format tests'
do
  "$script_dir/check-commits.sh" --message "$subject"
done

for subject in \
  'docs: explain parser' \
  'feat!: break output' \
  'Feat: add parser' \
  'feat add parser'
do
  if "$script_dir/check-commits.sh" --message "$subject" >/dev/null 2>&1; then
    printf 'unexpected valid commit subject: %s\n' "$subject" >&2
    exit 1
  fi
done

for version in \
  'v0.1.0' \
  'v1.2.3-rc.1' \
  'v1.2.3-alpha-beta+build.01'
do
  "$script_dir/check-version.sh" "$version"
done

for version in \
  '1.2.3' \
  'v01.2.3' \
  'v1.02.3' \
  'v1.2.03' \
  'v1.2.3-01' \
  'v1.2'
do
  if "$script_dir/check-version.sh" "$version" >/dev/null 2>&1; then
    printf 'unexpected valid release tag: %s\n' "$version" >&2
    exit 1
  fi
done

policy_test_dir=$(mktemp -d "${TMPDIR:-/tmp}/mori-policy.XXXXXX")
trap 'rm -rf -- "$policy_test_dir"' EXIT
git -C "$policy_test_dir" init -q
git -C "$policy_test_dir" config user.name "Policy Test"
git -C "$policy_test_dir" config user.email "policy@example.invalid"
git -C "$policy_test_dir" commit -q --allow-empty -m 'chore: seed policy test' -m 'generated body'

if "$script_dir/check-commits.sh" -C "$policy_test_dir" HEAD >/dev/null 2>&1; then
  printf 'commit body unexpectedly passed strict policy\n' >&2
  exit 1
fi
"$script_dir/check-commits.sh" --allow-body -C "$policy_test_dir" HEAD
