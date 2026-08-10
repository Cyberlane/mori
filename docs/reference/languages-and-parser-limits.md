# Languages and parser limits

Run `mori languages` for the authoritative capabilities of the installed
binary.

| Parser language | Review family | Domain | File types | Shebangs |
| --- | --- | --- | --- | --- |
| Bash / POSIX shell | shell | code | `.sh`, `.bash` | `sh`, `dash`, `bash` |
| C | c-cpp | code | `.c`, `.h` | — |
| C++ | c-cpp | code | `.cc`, `.cpp`, `.cxx`, `.hh`, `.hpp`, `.hxx` | — |
| C# | csharp | code | `.cs` | — |
| Go | go | code | `.go` | — |
| Hack | php-hack | code | `.hack`, legacy marked `.php` | — |
| Java | java | code | `.java` | — |
| JavaScript / JSX | javascript | code | `.js`, `.jsx`, `.mjs`, `.cjs` | `node`, `nodejs` |
| PHP | php-hack | code | `.php`, `.phtml` | — |
| Python | python | code | `.py`, `.pyi` | `python`, `python3` |
| Rust | rust | code | `.rs` | — |
| Swift | swift | code | `.swift` | — |
| TypeScript / TSX | typescript | code | `.ts`, `.mts`, `.cts`, `.tsx` | — |
| Zsh | shell | code | `.zsh` | `zsh` |
| SQL | sql | sql-query | `.sql` | — |
| PostgreSQL | sql | sql-query | `.sql` with explicit dialect | — |

For extensionless files, Mori reads at most 256 bytes and recognizes bounded
direct or `/usr/bin/env` shebangs without executing an interpreter. Extensions
take precedence. Legacy Hack is the sole content-aware exception: `.php` is
selected as Hack only when the first line, or the line after one optional
shebang, begins with an exact `<?hh` header.

## Comparison units

Code parsers produce implemented function-like units. Nested functions are
independent and excluded from their parent fingerprint. Bodyless declarations
are not scored.

Shell files additionally produce one top-level `script` unit whose function
bodies are excluded. SQL parsers produce top-level query units. Functions,
scripts, queries, and opt-in statement blocks never compare across fragment
kinds or comparison domains.

## Language-specific boundaries

- Java: implemented methods, constructors, compact constructors, and lambdas.
- C: function definitions. `.h` defaults to C; use a C++-specific header suffix
  when C++ parsing is required.
- C++: function definitions and lambdas.
- C#: implemented methods, constructors, destructors, operators, accessors,
  local functions, anonymous methods, and lambdas.
- Swift: implemented functions, initializers, deinitializers, and closures.
  Protocol requirements, computed properties, accessors, and subscripts are
  not independent units.
- PHP: implemented functions, methods, anonymous functions, and arrow
  functions.
- Hack: implemented functions, methods, anonymous functions, and lambdas.

## Visible incompleteness

Tree-sitter recovery is reported as warnings. Any fragment containing a parse
error is skipped with explicit diagnostic and coverage counts.

Mori has bounded, byte-preserving compatibility adaptations for several
recognized Swift forms and the upstream raw-ampersand JSX-text grammar issue.
Other unsupported syntax remains visibly incomplete. The pinned Zsh grammar
requires `:` for several glob-qualifier forms.

The Hack parser is a checksum-pinned MIT snapshot from the archived
`slackhq/tree-sitter-hack` repository. Newer Hack syntax may produce visible
diagnostics until Mori deliberately maintains or replaces that grammar.

Generic SQL extensions can produce diagnostics. The PostgreSQL parser targets
PostgreSQL 18.3 but does not extract PL/pgSQL bodies. Embedded SQL recognizes
only bounded direct Go string arguments and does not establish receiver types
or runtime contents.

Statement blocks are fixed-size syntax windows, not semantic regions. Low
token floors can make them noisy.
