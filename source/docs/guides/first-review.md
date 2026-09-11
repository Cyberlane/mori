# Choose a useful first review

> Requires Mori v0.33.0 or later for library suggestions, growing test-pattern
> policy and next-command guidance. Check `mori version` before following them.
> Earlier releases can use explicit roots and manually configured named scopes.

A broad scan can surface hundreds of similar test callbacks. This is often a
scope problem. A high structural score does not establish that two functions
behave alike or should be merged.

## Start with the code you maintain

For a no-write trial that also works with v0.32.0, choose a real source directory:

```sh
mori scan --profile review packages/core/src
```

Replace that path with your project's source root. The review profile uses
same-language comparisons, an 85% threshold and a 40-token floor. It does not
exclude tests. Colocated tests may still dominate this trial.

For a reusable policy, run setup from the project root:

```sh
mori setup
```

Choose a scope after inspecting its roots, exclusions, counts and sample paths:

| Scope | Use it for | What remains outside the promise |
| --- | --- | --- |
| `library` | Observed conventional library source roots | Custom layouts may need edited roots. Naming does not establish ownership. |
| `application` | Broad source with conventional tests and stories excluded | Demos, examples and tooling can remain. |
| `tests` | Observed test and story paths | Inline tests and future files outside these literal roots may be missing. |
| `all` | Inclusive exploration under existing discovery rules | Ignored, unsupported and generated-excluded files still follow policy. |

Run the exact scan command printed after setup. Named scopes are opt-in. Running
plain `mori scan` does not automatically select the scope you just created.
The printed command includes the configuration path so scope roots resolve
correctly even when setup targeted a different directory.

From the project root, a typical library review is:

```sh
mori scan --scope library --format agent --output review.json
```

Do not add `.` after this command unless you intend to replace the library's
suggested roots with the whole current directory. Positional roots override the
scope's roots, while its exclusions still apply.

For an existing `.mori.json`, use `mori configure`. Existing named scopes are not
overwritten implicitly. Choose a new scope name or explicitly edit the old one.
Top-level exclusions still apply, and exploratory scopes do not narrow the
canonical staged gate.

## Keep the scope useful as the repository grows

Application and library suggestions generalize observed conventions. For example,
one `thing.test.ts` produces `**/*.test.*`, so a later `other.test.ts` is covered.
A `tests` directory produces `**/tests/**`. Only observed conventions are proposed.
A new convention, such as stories in a previously test-only project, needs review.
Directories such as `runtime-test` remain eligible because they are not an exact
`test` or `tests` directory component.

Inspect patterns before applying them. They express naming policy and can exclude
production files that use those names. Inline tests need fragment selection where
supported. Keep a separate inclusive or test scope when reviewing test quality.

Human-readable reports show a scope hint when at least half of five or more
leading retained groups contain conventional test/story paths. The sample stops
at 25 groups. It is a navigation aid, not a false-positive rate, and it never
changes the scan or its JSON schema.

## Check what survived

Retain the report and inspect coverage, warnings, truncation, and both source
locations. Confirm that known relevant matches remain. A smaller result count
alone does not demonstrate better detection.

In controlled public-repository comparisons, guided application scope reduced
React Spring's core review from 119 groups to two. React Three Fiber's root
review fell from 63 to 14, of which three were library-source pairs and eleven
were demos or tooling. Those cases motivated the separate library scope. These
numbers describe frozen research inputs, not every revision or project's result.
The updated library scope returns four groups across React Spring's recognized
source roots and three in React Three Fiber. All previously checked source pairs
survived, and adding new test files after setup left both results unchanged.
React Spring's `targets/*/src` is a custom layout outside these automatic roots
and requires explicit selection when relevant. Some remaining matches are
intentional symmetry, such as pause/resume methods.

These checks used [React Spring at 4be40b8](https://github.com/pmndrs/react-spring/tree/4be40b8b590238dc80872a48b91a42d92201d895)
and [React Three Fiber at ff3899d](https://github.com/pmndrs/react-three-fiber/tree/ff3899dbf43d2a88895fecf53c147192abfd7431).

See [Reviewing results](reviewing-results.md) for deciding what deserves action
and [Scan selection](../scan-selection.md) for detailed policy precedence.
