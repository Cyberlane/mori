# Building and publishing documentation

The documentation site renders the repository's Markdown using pinned
Python-Markdown. Markdown files remain the source of truth. The build includes
all pages under `docs/`, root overview/contribution/changelog pages, and locally
linked Markdown references. Every page links to its exact Markdown snapshot included in the site. Other
repository-file links point to GitHub source.

## Build and validate

Use Python 3.10 or newer. Create the virtual environment and output outside the
checkout; these commands work on macOS and Linux:

```sh
python3 -m venv /tmp/mori-docs-venv
/tmp/mori-docs-venv/bin/python -m pip install -r scripts/docs-requirements.txt
/tmp/mori-docs-venv/bin/python scripts/test-build-docs.py
site_output=$(mktemp -d /tmp/mori-docs-site.XXXXXX)
/tmp/mori-docs-venv/bin/python scripts/build-docs.py --output "$site_output" --source-ref main
```

The builder refuses to overwrite a nonempty directory or place output inside the
checkout. It verifies every local page, image, stylesheet link and heading
fragment, and writes `build-report.json` with page/link counts and the renderer
version. Missing links fail the build. External source and release links are not
fetched by this check; validate the chosen source ref separately.

For a reproducible published snapshot, pass the exact source commit to
`--source-ref` instead of `main`, and build from that commit's clean checkout.
A development worktree build can preview uncommitted documentation. Each page
includes its exact Markdown source locally; GitHub code links use the chosen ref
and cannot represent uncommitted implementation changes.
The builder adds no timestamps to generated pages.

Preview with a local HTTP server:

```sh
/tmp/mori-docs-venv/bin/python -m http.server 8765 --directory "$site_output"
```

Check the overview, first-review guide, a wide reference table, and navigation
at desktop and phone widths. Confirm keyboard navigation, code scrolling,
images, anchors and the minimum-version banner. Stop the server after review.

## Publish deliberately

Publish only the validated output to an isolated `gh-pages` branch or equivalent
static hosting artifact. Configure GitHub Pages to serve that branch's root.
Keep source checkout changes, private files, research evidence and credentials
out of the publication tree. Review the artifact and obtain the normal project
publication authorization before pushing it. Do not mix publication history with
`main` or use deployment as evidence that a release was published.

The current template labels the site **Mori v0.34.0 documentation**. Publish it
only after that release is available, and retain the minimum-version notice.
Preserve that banner while the site describes unreleased behavior. Update it only
when the corresponding release is verified, and make the documentation version
explicit. There is intentionally no automatic main-branch or release-triggered
publishing workflow.

To change presentation, edit [the stylesheet](../site/site.css) and the HTML template in
[the builder](../../scripts/build-docs.py). Keep renderer updates pinned in
[the requirements file](../../scripts/docs-requirements.txt) and rerun the tests and visual checks after any
update.

The [build tests](../../scripts/test-build-docs.py) exercise real documentation,
source snapshots, Unicode headings, broken anchors and output-directory safety.
