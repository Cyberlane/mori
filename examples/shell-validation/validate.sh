#!/bin/sh

validate_email() {
  value=$1
  if [ -z "$value" ]; then
    return 1
  fi
  case "$value" in
    *@*.*) return 0 ;;
    *) return 1 ;;
  esac
}
