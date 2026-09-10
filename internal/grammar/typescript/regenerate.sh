#!/bin/sh
set -eu
cp common/scanner.h typescript/scanner.h
cp common/scanner.h tsx/scanner.h
case "$(tree-sitter --version)" in
  "tree-sitter 0.25.10"|"tree-sitter 0.25.10 ("*) ;;
  *) echo "tree-sitter 0.25.10 required" >&2; exit 1 ;;
esac
(cd typescript && tree-sitter generate --abi 14 && cp src/parser.c parser.c)
(cd tsx && tree-sitter generate --abi 14 && cp src/parser.c parser.c)
echo "Generated parser.c files updated. Verify checksums and remove disposable src output after inspection."
