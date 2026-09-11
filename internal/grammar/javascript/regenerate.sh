#!/bin/sh
set -eu
case "$(tree-sitter --version)" in
  "tree-sitter 0.25.10"|"tree-sitter 0.25.10 ("*) ;;
  *) echo "tree-sitter 0.25.10 required" >&2; exit 1 ;;
esac
(cd . && tree-sitter generate --abi 15 && cp src/parser.c parser.c)
echo "Generated parser.c files updated. Verify checksums and remove disposable src output after inspection."
