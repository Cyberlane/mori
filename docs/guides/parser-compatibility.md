# Parser compatibility

Mori parses source syntax without running a compiler, database, preprocessor,
or project build. A successful parse is comparison coverage, not proof that
code compiles, executes, or behaves like a matched fragment.

## C frameworks

The bundled C grammar preserves conditional initializer branches when entries
end in commas. It retains both alternatives without selecting a build
configuration. It also recognizes these declaration and iteration forms:

- `static UPPERCASE_DEFINE(...);` declarations with ordinary expression arguments.
- `DT_INST_FOREACH_STATUS_OKAY(...)` declarations.
- `ZMK_DISPLAY_WIDGET_LISTENER(name, type, update, get)` declarations.
- `STRUCT_SECTION_FOREACH(type_name, variable) { ... }` statement bodies.
- One `else` clause enclosed by `#if`, `#ifdef`, or `#ifndef` after an `if` with a braced body and no existing `else`.

These grammar rules retain arguments and bodies; they do not expand macros or
validate macro definitions and arity. Other macro dialects and conditional
layouts may still produce visible diagnostics. Invalid initializer branches,
malformed arguments, and missing delimiters remain diagnosed, and affected
functions are excluded from scoring.

## Swift

The bundled grammar distinguishes adjacent optional-type markers (`T??`) from
whitespace-separated nil coalescing (`value as? T ?? fallback`). This also works
inside call arguments, including subscript and method-call cast operands.
The original cast and coalescing syntax is preserved without inserting synthetic
parentheses. Malformed cast types, missing operands, and broken neighboring
syntax remain diagnosed. Other inherited grammar limitations may still apply.

Normalization version 13 invalidates older accepted identities because removing
synthetic grouping changes profiles of expressions previously repaired that way.
Review those identities again using the project upgrade and baseline workflow;
do not copy old fingerprints into a new baseline to bypass the change.

## SQL

See [SQL and embedded SQL](sql.md) for bounded SQLite compatibility and why a
valid schema-only file may contain no comparable fragments.

Generated grammar sources, local modifications, licenses, ABI versions, and
checksums are recorded alongside each grammar under `internal/grammar/`.
