#!/bin/zsh

check_address() {
  candidate=$1
  if [ -z "$candidate" ]; then
    return 1
  fi
  case "$candidate" in
    *@*.*) return 0 ;;
    *) return 1 ;;
  esac
}
