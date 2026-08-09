#!/bin/zsh

report_items() {
  for item in "$@"; do
    print -r -- "item: $item"
  done
}
