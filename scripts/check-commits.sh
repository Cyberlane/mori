#!/usr/bin/env bash
set -euo pipefail

pattern='^(feat|fix|chore|style)(\([[:alnum:]_.\/-]+\))?: .+$'

allow_body=false
git_directory=

while [[ $# -gt 0 ]]; do
  case $1 in
    --allow-body)
      allow_body=true
      shift
      ;;
    -C)
      if [[ $# -lt 2 ]]; then
        printf 'missing directory after -C\n' >&2
        exit 2
      fi
      git_directory=$2
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

git_command=(git)
if [[ -n $git_directory ]]; then
  git_command+=(-C "$git_directory")
fi

check_subject() {
  local subject=$1
  if [[ ! $subject =~ $pattern ]]; then
    printf 'invalid commit subject: %s\n' "$subject" >&2
    printf 'expected: feat|fix|chore|style(optional-scope): summary\n' >&2
    return 1
  fi
}

if [[ ${1:-} == "--message" ]]; then
  if [[ $# -ne 2 ]]; then
    printf 'usage: %s --message "subject"\n' "$0" >&2
    exit 2
  fi
  check_subject "$2"
  exit
fi

if [[ $# -ne 1 ]]; then
  printf 'usage: %s [--allow-body] [-C directory] <git-revision-range>\n' "$0" >&2
  exit 2
fi

failed=0
while IFS= read -r commit; do
  subject=$("${git_command[@]}" show -s --format=%s "$commit")
  body=$("${git_command[@]}" show -s --format=%b "$commit")
  if ! check_subject "$subject"; then
    printf 'commit: %s\n' "$commit" >&2
    failed=1
  fi
  if [[ $allow_body == false && -n ${body//[[:space:]]/} ]]; then
    printf 'commit %s has a body; commit messages must be one line\n' "$commit" >&2
    failed=1
  fi
done < <("${git_command[@]}" rev-list --reverse "$1")

exit "$failed"
