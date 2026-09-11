#!/usr/bin/env python3
"""Run with the same pinned virtual environment as build-docs.py."""
import importlib.util
from pathlib import Path
import tempfile
import sys
sys.dont_write_bytecode = True
import unittest

spec = importlib.util.spec_from_file_location("build_docs", Path(__file__).with_name("build-docs.py"))
docs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(docs)


class BuildDocsTest(unittest.TestCase):
    def test_repository_site_preserves_links_assets_and_release_status(self):
        with tempfile.TemporaryDirectory(prefix="mori-docs-test-") as directory:
            output = Path(directory)
            report = docs.build(docs.ROOT, output, "main")
            self.assertGreaterEqual(report["html_pages"], 20)
            self.assertGreater(report["local_links_checked"], 100)
            self.assertEqual(report["broken_links"], 0)
            self.assertTrue((output / "docs/guides/first-review.html").is_file())
            self.assertTrue((output / "docs/assets/mori-hero.webp").is_file())
            self.assertEqual((output / "source/docs/guides/first-review.md").read_bytes(), (docs.ROOT / "docs/guides/first-review.md").read_bytes())
            overview = (output / "index.html").read_text()
            self.assertIn('href="docs/getting-started.html#run-a-first-review"', overview)
            self.assertIn('src="docs/assets/mori-hero.webp"', overview)
            self.assertIn('https://github.com/Cyberlane/mori/blob/main/LICENSE', overview)
            for page in output.rglob("*.html"):
                self.assertIn("Markdown source", page.read_text())
                self.assertIn("Mori v0.34.0 documentation", page.read_text())
                self.assertIn("v0.33.0 or later", page.read_text())
            page = output / "index.html"
            page.write_text(page.read_text() + '<a href="docs/getting-started.html#missing-heading">Broken</a>')
            with self.assertRaisesRegex(ValueError, "Missing site anchor"):
                docs.verify(output)

    def test_refuses_to_overwrite_or_write_in_repository(self):
        with self.assertRaisesRegex(ValueError, "outside"):
            docs.build(docs.ROOT, docs.ROOT / "site-output", "main")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            (path / "keep").write_text("untouched")
            with self.assertRaisesRegex(ValueError, "empty"):
                docs.build(docs.ROOT, path, "main")
            self.assertEqual((path / "keep").read_text(), "untouched")

    def test_unicode_heading_ids_and_duplicate_headings(self):
        body, _ = docs.render("# 森 Mori\n\n## Usage & examples\n\n## Usage & examples")
        self.assertIn('id="森-mori"', body)
        self.assertIn('id="usage--examples"', body)
        self.assertIn('id="usage--examples_1"', body)

    def test_rejects_links_outside_repository(self):
        with self.assertRaisesRegex(ValueError, "escapes repository"):
            docs.local_target(Path("README.md"), "../private.md")


if __name__ == "__main__":
    unittest.main()
