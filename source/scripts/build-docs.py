#!/usr/bin/env python3
"""Render repository Markdown into a checked, dependency-pinned static site.

Install scripts/docs-requirements.txt in a virtual environment, then run:
  python scripts/build-docs.py --output /tmp/mori-docs-site
Output must be an empty directory outside the repository. No publishing occurs.
"""
from __future__ import annotations

import argparse
import html
from html.parser import HTMLParser
import json
import os
from pathlib import Path
import re
import shutil
from urllib.parse import quote, unquote, urlsplit, urlunsplit

import markdown

RENDERER_VERSION = "3.8.2"
ROOT = Path(__file__).resolve().parents[1]


def slugify(value: str, separator: str) -> str:
    """GitHub-style Unicode heading IDs; Markdown's toc handles duplicates."""
    value = value.lower().strip()
    value = re.sub(r"[^\w\- ]", "", value, flags=re.UNICODE)
    return value.replace(" ", separator)


def page_path(source: Path) -> Path:
    if source.name.lower() == "readme.md":
        return source.with_name("index.html")
    return source.with_suffix(".html")


class Links(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=False)
        self.links: list[str] = []
        self.ids: set[str] = set()

    def handle_starttag(self, tag, attrs):
        for key, value in attrs:
            if key in {"href", "src"} and value:
                self.links.append(value)
            if key == "id" and value:
                self.ids.add(value)

    handle_startendtag = handle_starttag


def render(text: str) -> tuple[str, str]:
    renderer = markdown.Markdown(
        extensions=["fenced_code", "tables", "toc", "sane_lists"],
        extension_configs={"toc": {"slugify": slugify, "toc_depth": "2-3"}},
    )
    return renderer.convert(text), renderer.toc


def local_target(source: Path, url: str) -> Path | None:
    parts = urlsplit(url)
    if parts.scheme or parts.netloc or not parts.path:
        return None
    if parts.path.startswith("/"):
        raise ValueError(f"Repository-absolute link is ambiguous: {source}: {url}")
    target = Path(os.path.normpath(source.parent / unquote(parts.path)))
    if target.is_absolute() or ".." in target.parts:
        raise ValueError(f"Link escapes repository: {source}: {url}")
    return target


class Rewrite(HTMLParser):
    def __init__(self, source, pages, root, output, source_url):
        super().__init__(convert_charrefs=False)
        self.source, self.pages, self.root, self.output = source, pages, root, output
        self.source_url = source_url
        self.result: list[str] = []

    def url(self, url):
        target = local_target(self.source, url)
        if target is None:
            return url
        parts = urlsplit(url)
        disk = self.root / target
        if not disk.exists():
            raise ValueError(f"Missing repository link: {self.source}: {url}")
        if target in self.pages:
            rewritten = os.path.relpath(page_path(target), page_path(self.source).parent)
        elif target.as_posix() in {"scripts/build-docs.py", "scripts/test-build-docs.py", "scripts/docs-requirements.txt", "docs/site/site.css"}:
            destination = self.output / "source" / target
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(disk, destination)
            rewritten = os.path.relpath(Path("source") / target, page_path(self.source).parent)
        elif target.parts[:2] == ("docs", "assets") and disk.is_file():
            destination = self.output / target
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(disk, destination)
            rewritten = os.path.relpath(target, page_path(self.source).parent)
        else:
            return self.source_url + "/" + quote(target.as_posix(), safe="/") + (
                "#" + parts.fragment if parts.fragment else ""
            )
        return urlunsplit(("", "", quote(Path(rewritten).as_posix(), safe="/.."), parts.query, parts.fragment))

    def start(self, tag, attrs, closed=False):
        rendered = []
        for key, value in attrs:
            if value is not None and key in {"href", "src"}:
                value = self.url(value)
            rendered.append(key if value is None else f'{key}="{html.escape(value, quote=True)}"')
        self.result.append("<" + tag + (" " + " ".join(rendered) if rendered else "") + (" />" if closed else ">"))

    def handle_starttag(self, tag, attrs):
        self.start(tag, attrs)

    def handle_startendtag(self, tag, attrs):
        self.start(tag, attrs, True)

    def handle_endtag(self, tag):
        self.result.append(f"</{tag}>")

    def handle_data(self, data):
        self.result.append(data)

    def handle_entityref(self, name):
        self.result.append(f"&{name};")

    def handle_charref(self, name):
        self.result.append(f"&#{name};")

    def handle_comment(self, data):
        self.result.append(f"<!--{data}-->")


def discover(root: Path) -> dict[Path, tuple[str, str]]:
    pending = {Path("README.md"), Path("CONTRIBUTING.md"), Path("CHANGELOG.md")}
    pending.update(path.relative_to(root) for path in (root / "docs").rglob("*.md"))
    pages = {}
    while pending:
        source = min(pending)
        pending.remove(source)
        if source in pages:
            continue
        body, toc = render((root / source).read_text())
        pages[source] = (body, toc)
        links = Links()
        links.feed(body)
        for url in links.links:
            target = local_target(source, url)
            if target is not None and target.suffix.lower() == ".md" and target not in pages:
                pending.add(target)
    return pages


def verify(output: Path) -> dict:
    output = output.resolve()
    parsed = {}
    for page in sorted(output.rglob("*.html")):
        links = Links()
        links.feed(page.read_text())
        parsed[page.resolve()] = links
    checked = 0
    for page, links in parsed.items():
        for url in links.links:
            parts = urlsplit(url)
            if parts.scheme or parts.netloc:
                continue
            target = (page.parent / unquote(parts.path)).resolve() if parts.path else page
            if not target.is_relative_to(output.resolve()) or not target.is_file():
                raise ValueError(f"Broken site link: {page.relative_to(output)}: {url}")
            if parts.fragment and target in parsed and unquote(parts.fragment) not in parsed[target].ids:
                raise ValueError(f"Missing site anchor: {page.relative_to(output)}: {url}")
            checked += 1
    return {"html_pages": len(parsed), "local_links_checked": checked, "broken_links": 0}


def build(root: Path, output: Path, source_ref: str) -> dict:
    if markdown.__version__ != RENDERER_VERSION:
        raise ValueError(f"Install pinned Markdown=={RENDERER_VERSION}; found {markdown.__version__}")
    root, output = root.resolve(), output.resolve()
    if output.is_relative_to(root) or root.is_relative_to(output):
        raise ValueError("Output must be outside the repository and cannot be its ancestor")
    if output.exists() and any(output.iterdir()):
        raise ValueError("Output directory must be empty; choose a fresh temporary directory")
    output.mkdir(parents=True, exist_ok=True)
    source_url = "https://github.com/Cyberlane/mori/blob/" + quote(source_ref, safe="")
    pages = discover(root)
    titles = {}
    for source in pages:
        text = (root / source).read_text()
        match = re.search(r"^#\s+(.+)$", text, re.MULTILINE)
        titles[source] = re.sub(r"[`*_]", "", match.group(1)) if match else source.stem
    css = output / "site.css"
    shutil.copyfile(root / "docs/site/site.css", css)
    for source, (body, toc) in sorted(pages.items()):
        target = output / page_path(source)
        target.parent.mkdir(parents=True, exist_ok=True)
        source_copy = output / "source" / source
        source_copy.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(root / source, source_copy)
        rewrite = Rewrite(source, pages, root, output, source_url)
        rewrite.feed(body)
        relative = lambda path: quote(Path(os.path.relpath(path, page_path(source).parent)).as_posix(), safe="/..")
        nav = []
        priority = [Path("docs/README.md"), Path("docs/guides/first-review.md"), Path("docs/getting-started.md"), Path("README.md")]
        for item in sorted(pages, key=lambda p: (priority.index(p) if p in priority else len(priority), p.as_posix())):
            label = "Overview" if item == Path("README.md") else titles[item]
            active = ' aria-current="page"' if item == source else ""
            nav.append(f'<a href="{relative(page_path(item))}"{active}>{html.escape(label)}</a>')
        target.write_text(f'''<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{html.escape(titles[source])} · Mori documentation</title>
<meta name="description" content="Mori structural similarity documentation, usage guides and references.">
<link rel="stylesheet" href="{relative(Path('site.css'))}"></head>
<body><a class="skip" href="#content">Skip to content</a>
<header><a class="brand" href="{relative(Path('index.html'))}"><span aria-hidden="true">森</span> Mori <small>Documentation</small></a><a href="https://github.com/Cyberlane/mori">GitHub ↗</a></header>
<div class="development" role="note"><strong>Mori v0.33.0 documentation</strong> · Library scopes and support bundles require <strong>v0.33.0 or later</strong>. Check <a href="https://github.com/Cyberlane/mori/releases/latest">release notes</a> against your installed version.</div>
<div class="layout"><aside><details open><summary>Browse documentation</summary><nav aria-label="Documentation">{''.join(nav)}</nav></details></aside>
<main id="content"><div class="page-meta">GUIDES &amp; REFERENCE <a href="{relative(Path('source') / source)}">Markdown source ↗</a></div>
{''.join(rewrite.result)}
<footer>Scores describe structural similarity; they do not prove behavioral equivalence.<br>Source stays local. <a href="https://github.com/Cyberlane/mori">Mori on GitHub</a></footer></main>
<aside class="contents"><strong>On this page</strong>{toc}</aside></div><script>const menu=document.querySelector("aside details"); const narrow=matchMedia("(max-width:900px)"); function sizeMenu(){{menu.open=!narrow.matches;}} sizeMenu(); narrow.addEventListener("change",sizeMenu);</script></body></html>''')
    (output / ".nojekyll").write_text("")
    result = verify(output)
    result.update(renderer=f"Python-Markdown {RENDERER_VERSION}", source_ref=source_ref)
    (output / "build-report.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--source-ref", default="main", help="Git ref for upstream source links")
    args = parser.parse_args()
    print(json.dumps(build(ROOT, args.output, args.source_ref), indent=2))


if __name__ == "__main__":
    main()
