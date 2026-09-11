#!/usr/bin/env python3
"""Exercise acceptance gates with a real binary and deliberately damaged baselines."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


class AcceptanceGateTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="mori-gate-tests-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.root = Path(cls.temporary.name)
        cls.command = [sys.executable, str(Path(__file__).with_name("acceptance.py")),
                       "--binary", str(Path("bin/mori").resolve()), "--size", "3", "--repeats", "5",
                       "--runner-id", "gate-test", "--output", str(cls.root / "actual.json")]
        result = subprocess.run(cls.command, capture_output=True, text=True, timeout=30)
        if result.returncode:
            raise AssertionError(result.stdout + result.stderr)
        cls.baseline = json.loads((cls.root / "actual.json").read_text())

    def run_baseline(self, baseline, *options):
        path = self.root / "baseline.json"
        path.write_text(json.dumps(baseline))
        result = subprocess.run(self.command + ["--baseline", str(path), *options],
                                capture_output=True, text=True, timeout=30)
        artifact = json.loads((self.root / "actual.json").read_text())
        return result, artifact

    def test_unchanged_findings_pass(self):
        memory_options = [] if self.baseline["workloads"]["repeated"]["memory_measurement"] == "unavailable" else ["--memory-gate"]
        result, artifact = self.run_baseline(self.baseline, "--timing-gate", "--max-regression", "100",
                                             "--max-memory-regression", "100", *memory_options)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(artifact["result_changes"], [])

    def test_changed_score_requires_review(self):
        baseline = copy.deepcopy(self.baseline)
        baseline["workloads"]["repeated"]["identities"][0]["score"] = 0.123
        result, artifact = self.run_baseline(baseline)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(artifact["result_changes"])
        result, artifact = self.run_baseline(baseline, "--allow-result-changes")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(artifact["result_changes"])

    def test_changed_counts_require_review(self):
        baseline = copy.deepcopy(self.baseline)
        baseline["workloads"]["repeated"]["counts"]["fragments"] += 1
        result, artifact = self.run_baseline(baseline)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(artifact["result_changes"])

    def test_toolchain_mismatch_rejected(self):
        baseline = copy.deepcopy(self.baseline)
        baseline["toolchain"]["go_version"] = "different-toolchain"
        result, artifact = self.run_baseline(baseline)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("incomparable benchmark environments", artifact["violations"])

    def test_time_regression_rejected(self):
        baseline = copy.deepcopy(self.baseline)
        for workload in baseline["workloads"].values():
            workload["median_ms"] = 0.000001
        result, artifact = self.run_baseline(baseline, "--timing-gate")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(any("time increased" in item for item in artifact["violations"]))

    def test_memory_regression_rejected_when_available(self):
        baseline = copy.deepcopy(self.baseline)
        if baseline["workloads"]["repeated"]["memory_measurement"] == "unavailable":
            self.skipTest("native per-process memory measurement unavailable")
        for workload in baseline["workloads"].values():
            workload["peak_rss_bytes"] = [1] * 5
        result, artifact = self.run_baseline(baseline, "--memory-gate")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(any("memory increased" in item for item in artifact["violations"]))

    def test_missing_memory_rejected_when_gated(self):
        baseline = copy.deepcopy(self.baseline)
        if baseline["workloads"]["repeated"]["memory_measurement"] == "unavailable":
            self.skipTest("native per-process memory measurement unavailable")
        baseline["workloads"]["repeated"]["peak_rss_bytes"] = [None] * 5
        result, artifact = self.run_baseline(baseline, "--memory-gate")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(any("memory measurements unavailable" in item for item in artifact["violations"]))


if __name__ == "__main__":
    unittest.main()
