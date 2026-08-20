# Cross-language and language boundaries

Read this reference for deliberate cross-language exploration or when parser
boundaries affect the review. Start at threshold `0.65` and
`--min-tokens 40`; lower the floor toward 12 only for explicitly broad
exploration, accepting more callbacks, wrappers, and boilerplate.
Set `MORI_REPORT` to the owner-private report path selected under the shared
review contract before using these examples.

Choose exactly one filtering mode. Broad family selection:

```sh
mori scan \
  --comparison-domain code \
  --cross-language-only \
  --require-coverage \
  --threshold 0.65 \
  --min-tokens 40 \
  --format agent \
  --output "$MORI_REPORT" \
  .
```

For one pairing:

```sh
mori scan \
  --comparison-domain code \
  --language-pair go,typescript \
  --require-coverage \
  --threshold 0.65 \
  --min-tokens 40 \
  --format agent \
  --output "$MORI_REPORT" \
  .
```

Never combine `--cross-language-only` with `--same-language-only` or
`--language-pair`. TypeScript and TSX are one family and do not count as
cross-language. Bash/POSIX shell and Zsh are the `shell` family; use
`--language-pair bash,zsh` for a dialect-only review. PHP and Hack are the
`php-hack` family; use `--language-pair php,hack` for that dialect pairing.
Modern Hack uses `.hack`; legacy `.php` is selected as Hack only from an
exact bounded `<?hh` header on the first line or after one optional shebang.

## Fragment and parser boundaries

- Shell files produce a `script` unit for top-level executable statements and
  separate `function` units for named functions, each subject to the token
  floor. Require both kinds when claiming shell-file coverage: function-only
  results omit top-level orchestration and script results omit function bodies.
- Java covers implemented methods, constructors, compact constructors, and
  lambdas; bodyless methods are unexamined.
- C# covers implemented methods, constructors, destructors, operators,
  accessors, local functions, anonymous methods, and lambdas; bodyless members
  are unexamined.
- PHP covers implemented functions, methods, anonymous functions, and arrow
  functions. Hack covers implemented functions, methods, anonymous functions,
  and lambdas. Bodyless declarations are unexamined.
- Swift covers implemented functions, initializers, deinitializers, and
  closures. Protocol requirements, computed properties, accessors, and
  subscripts are unexamined comparison units.

Treat type checking, overload resolution, dispatch, exception behavior, and
effects as source-review concerns, not conclusions from canonical syntax.
Disclose parser warnings as incomplete coverage. The pinned Hack grammar
upstream is archived, so inspect newer Hack syntax directly. Mori applies
bounded compatibility repairs for several recognized valid Swift forms. A
repaired optional-await binding retains the bound expression but omits its
`try? await` wrapper from structural features; inspect async and error-handling
behavior in source. Disclose any Swift parser warnings too.

## Review boundaries

Keep the full bounded report as evidence and inspect at most 25 distinct
content identities deeply unless exhaustive work is explicitly requested.
Open both ranges and surrounding context, including the source-language
constructs that canonicalization cannot represent. Do not lower the token
floor, bypass `.mori.json`, or broaden ignores merely to produce a match.
Classify results as likely duplication, intentional similarity, or false
positive; a cross-language score is never semantic equivalence.
