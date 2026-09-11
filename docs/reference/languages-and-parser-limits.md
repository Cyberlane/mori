# Languages and parser limits

Run `mori languages` for the authoritative capabilities of the installed
binary.

| Parser language | Review family | Domain | File types | Shebangs |
| --- | --- | --- | --- | --- |
| Bash / POSIX shell | shell | code | `.sh`, `.bash` | `sh`, `dash`, `bash` |
| C | c-cpp | code | `.c`, `.h` | — |
| C++ | c-cpp | code | `.cc`, `.cpp`, `.cxx`, `.hh`, `.hpp`, `.hxx` | — |
| C# | csharp | code | `.cs` | — |
| Dart | dart | code | `.dart` | — |
| Go | go | code | `.go` | — |
| GDScript | gdscript | code | `.gd` | — |
| Hack | php-hack | code | `.hack`, legacy marked `.php` | — |
| Java | java | code | `.java` | — |
| JavaScript / JSX | javascript | code | `.js`, `.jsx`, `.mjs`, `.cjs` | `node`, `nodejs` |
| Kotlin | kotlin | code | `.kt`, `.kts` | `kotlin` |
| Lua | lua-luau | code | `.lua` | `lua`, `lua5.1`–`lua5.4`, `luajit` |
| Luau | lua-luau | code | `.luau` | — |
| PHP | php-hack | code | `.php`, `.phtml` | — |
| PowerShell | powershell | code | `.ps1`, `.psd1`, `.psm1` | `powershell`, `pwsh` |
| Python | python | code | `.py`, `.pyi` | `python`, `python3` |
| Ruby | ruby | code | `.rb`, `.rake`, `.gemspec` | `ruby` |
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
- Lua and Luau: named/local function declarations and anonymous function
  definitions. They share a review family but retain distinct concrete IDs.
- GDScript: function definitions and lambdas. C# files in Godot projects are
  handled by the existing C# grammar; Mori does not parse Godot scenes or
  resources as code.
- Kotlin: function declarations, anonymous functions, and lambdas.
- Ruby: instance methods, singleton methods, and lambdas.
- Dart: named function bodies, local functions, and anonymous function
  expressions. The grammar exposes a named function's signature beside its
  body, so the body is scored and named from that adjacent signature; parameter
  declarations are not part of that fragment's fingerprint.
- PowerShell: functions and class methods. Script blocks passed as ordinary
  command arguments are not independent units.
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
Additional bounded grammar corrections cover JavaScript/TypeScript import types,
semicolonless generic interface overloads, keyword export aliases, and Java
annotated varargs; see [parser compatibility](../guides/parser-compatibility.md).
The maintained Go grammar accepts Go 1.26 `new(expression)` alongside existing
`new(type)` and `make(type, ...)` forms. TypeScript/TSX accepts explicit `in`,
`out`, and `in out` variance annotations. Swift accepts parenthesized expression
range patterns such as `case (n / 2 + 1)...:`. These grammar changes preserve
original source locations and keep nearby malformed syntax visible.

C compatibility includes the named libgit2 `GIT_FORMAT_PRINTF` annotation and
compound bodies for `git_vector_foreach` and
`git_attr_file__foreach_matching_rule`. Mori parses their arguments and bodies
without expanding macros or executing a preprocessor. This bounded support does
not establish coverage of arbitrary C macros or generated functions.

Flow is not a supported JavaScript dialect. For a JavaScript file with a leading
`@flow` comment and parse errors, the warning names the unsupported dialect and
retains diagnostic and skipped-fragment counts. Remaining valid JavaScript
fragments can still be compared, but do not establish complete Flow coverage.
Use `--fail-on-parse-diagnostic` when partial parsing must fail the scan.
Other unsupported syntax remains visibly incomplete. The pinned Zsh grammar
requires `:` for several glob-qualifier forms.

The Hack parser is a checksum-pinned MIT snapshot from the archived
`slackhq/tree-sitter-hack` repository. Newer Hack syntax may produce visible
diagnostics until Mori deliberately maintains or replaces that grammar.

The GDScript parser is a checksum-pinned MIT snapshot of upstream tag `v6.1.0`.
Its generated C sources are kept in the repository so ordinary builds never
download or regenerate a grammar.

Generic SQL extensions can produce diagnostics. The PostgreSQL parser targets
PostgreSQL 18.3 but does not extract PL/pgSQL bodies. Embedded SQL recognizes
only bounded direct Go string arguments and does not establish receiver types
or runtime contents.

Statement blocks are fixed-size syntax windows, not semantic regions. Low
token floors can make them noisy.

## Test selection

`--fragment-selection all` keeps the default comparison universe. Opt-in
`production` and `tests` classify fragments using these explicit conventions:

- A path directory named `test`, `tests`, or `__tests__`.
- File suffixes `_test.go`, `_test.rs`, `_test.py`, `.test.js`, `.test.jsx`,
  `.test.ts`, `.test.tsx`, `.spec.js`, `.spec.jsx`, `.spec.ts`, or `.spec.tsx`;
  and Python files named `test_*.py`.
- Rust `#[test]`, `#[tokio::test]`, and `#[async_std::test]` attributes, including
  arguments for the asynchronous test attributes.
- Rust `#[cfg(test)]` and compound predicates that necessarily require `test`,
  such as `all(test, not(loom))`. Every branch of an `any(...)` must require
  `test` before it is classified as test-only.

- C/C++ functions inside positive `#ifdef REDIS_TEST` or
  `#if defined(REDIS_TEST)` branches. The alternate branch, negative guard and
  disjunction with a production condition are not classified as tests.

Explicit classification path overrides take precedence over these conventions.
See [configuration](../configuration.md#classification-overrides).

`cfg(any(test, feature = "production"))`, `cfg(not(test))`, and a function name
merely containing `test` do not prove test-only membership. Unclassified units
remain on the production side. Path conventions apply to every fragment in the
file, including test helpers; this is a disclosed policy, not semantic inference.
Stories are not automatically classified as tests by fragment selection; setup
may separately suggest story-path scopes and exclusions.

Per-file exclusion counts remain distinct from parse failures and token-floor
counts. Files with no retained units show the `fragment_selection` zero-fragment
reason when selection (possibly combined with the token floor) explains the
omission. Review that evidence before asserting coverage or migrating a baseline.

Rust item macro token trees produce an explicit coverage warning; an otherwise
empty file reports `opaque_syntax`. They are not expanded into arbitrary function
bodies. Conventional path classification uses the current/selected root (the
repository root for index snapshots); a standalone external file has filename
and attribute evidence, not an inferred project directory. Component
files such as `.vue` and `.svelte`, and Zig source, remain outside the registered
parser set; inspect unsupported-extension counts separately from supported-file
coverage. A high supported-file coverage ratio does not measure those files.
