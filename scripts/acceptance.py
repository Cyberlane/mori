#!/usr/bin/env python3
"""Black-box Mori workloads. No dependencies or repository code execution."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import statistics
import signal
import subprocess
import tempfile
import time


def command(args, cwd, timeout=300):
    started = time.monotonic()
    if hasattr(os, "wait4"):
        with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
            process = subprocess.Popen(args, cwd=cwd, stdout=stdout, stderr=stderr)
            while True:
                pid, status, usage = os.wait4(process.pid, os.WNOHANG)
                if pid:
                    process.returncode = os.waitstatus_to_exitcode(status)
                    break
                if time.monotonic() - started > timeout:
                    process.kill()
                    _, status, _ = os.wait4(process.pid, 0)
                    process.returncode = os.waitstatus_to_exitcode(status)
                    raise subprocess.TimeoutExpired(args, timeout)
                time.sleep(0.005)
            stdout.seek(0)
            stderr.seek(0)
            result = subprocess.CompletedProcess(args, process.returncode, stdout.read().decode(), stderr.read().decode())
            result.peak_rss_bytes = usage.ru_maxrss * (1 if platform.system() == "Darwin" else 1024)
    else:
        result = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        result.peak_rss_bytes = None
    return result, round((time.monotonic() - started) * 1000, 3)


def generate(root, count):
    templates = [
        "let s=0; for(const x of xs) { if(x>0) s+=x; } return s;",
        "return xs.filter(x=>x.active).map(x=>x.value);",
        "let s=[]; for(let i=xs.length-1;i>=0;i--) { s.push(xs[i]); } return s;",
        "try { return JSON.parse(xs); } catch(e) { throw new Error('invalid'); }",
        "if(!xs) return null; return xs.key || xs.default;",
        "let i=0; while(i<xs.length && xs[i]!==null) { i++; } return i;",
    ]
    for name in ("repeated", "mixed", "malformed"):
        (root / name).mkdir()
    for name in ("repeated", "mixed"):
        for i in range(count):
            body = templates[0 if name == "repeated" else i % len(templates)]
            (root / name / f"f{i}.ts").write_text(f"export function f{i}(xs:any) {{ {body} }}\n")
    (root / "malformed" / "broken.ts").write_text("export function broken( { return ! !\n")
    (root / "malformed" / "good.ts").write_text("export function total(xs:any) { " + templates[0] + " }\n")
    return [root / name for name in ("repeated", "mixed", "malformed")]


def scan(binary, root, scratch, repeats, timeout, extra=()):
    samples, counts, digest, sessions, memory = [], None, None, [], []
    for iteration in range(repeats):
        session_path = scratch / f"session-{root.parent.name}-{root.name}-{iteration}.json"
        args = [str(binary), "scan", "--diagnostics", str(session_path), "--no-config",
                "--profile", "review", "--min-tokens", "12", "--max-pairs", "5000000",
                "--max-groups", "0", "--redact-paths", "--format", "json", *extra, str(root)]
        result, elapsed = command(args, scratch, timeout)
        if result.returncode != 0:
            raise AssertionError(f"scan {root.name}: exit {result.returncode}: {result.stderr[-1500:]}")
        session = json.loads(session_path.read_text())
        current_counts = session["counts"]
        current_digest = hashlib.sha256(result.stdout.encode()).hexdigest()
        if counts is not None and (counts != current_counts or digest != current_digest):
            raise AssertionError(f"non-deterministic report for {root.name}")
        counts, digest = current_counts, current_digest
        sessions.append(session)
        samples.append(elapsed)
        memory.append(result.peak_rss_bytes)
        parsed = json.loads(result.stdout)
        # Compare all identities and exact evidence, independent of ranking and paths.
        identities = sorted(({
            "id": group["content_pair_id"], "score": group["similarity"],
            "location_pairs": group["location_pairs"],
            "profiles": sorted([profile["fingerprint"], profile["occurrence_count"]]
                               for profile in group["profiles"]),
            "structural_evidence": group.get("structural_evidence"),
        } for group in parsed["groups"]), key=lambda item: item["id"])
    return {"samples_ms": samples, "median_ms": statistics.median(samples),
            "peak_rss_bytes": memory, "memory_measurement": "wait4" if hasattr(os, "wait4") else "unavailable",
            "counts": counts, "report_sha256": digest, "identities": identities, "sessions": sessions}


def cancellation(binary, scratch):
    if os.name == "nt":
        return {"status": "unavailable", "reason": "portable console interrupt delivery is not configured"}
    path = scratch / "cancellation.ts"
    path.write_text("\n".join(f"function f{i}(xs:any) {{ let s=0; for(const x of xs) {{ if(x>0) s+=x; }} return s; }}" for i in range(5000)))
    with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
        process = subprocess.Popen([str(binary), "scan", "--no-config", "--min-tokens", "12",
                                    "--max-pairs", "0", str(path)], cwd=scratch, stdout=stdout, stderr=stderr)
        try:
            time.sleep(0.2)
            if process.poll() is not None:
                assert process.returncode == 0, "cancellation workload failed before interrupt"
                return {"status": "completed_before_interrupt"}
            started = time.monotonic()
            process.send_signal(signal.SIGINT)
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                raise AssertionError("scan did not exit within 5 seconds of interrupt")
            assert process.returncode != 0, "interrupted scan reported success"
            return {"status": "cancelled", "latency_ms": round((time.monotonic() - started) * 1000, 3),
                    "exit_code": process.returncode, "interrupt_after_ms": 200}
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", default="bin/mori")
    parser.add_argument("--output", default="dist/acceptance.json")
    parser.add_argument("--size", type=int, default=80)
    parser.add_argument("--repeats", type=int, default=2)
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--public-corpus", action="store_true")
    parser.add_argument("--baseline", type=Path)
    parser.add_argument("--allow-result-changes", action="store_true",
                        help="explicitly permit reviewed finding/count changes, retaining the differences")
    parser.add_argument("--timing-gate", action="store_true")
    parser.add_argument("--memory-gate", action="store_true")
    parser.add_argument("--runner-id", default="uncontrolled")
    parser.add_argument("--max-regression", type=float, default=0.20)
    parser.add_argument("--max-memory-regression", type=float, default=0.20)
    args = parser.parse_args()
    if not 3 <= args.size <= 2000 or args.repeats < 2 or args.timeout < 1:
        parser.error("size must be 3..2000, repeats >= 2, timeout >= 1")
    if args.max_regression < 0 or args.max_memory_regression < 0:
        parser.error("regression tolerances must be nonnegative")
    if (args.timing_gate or args.memory_gate) and (not args.baseline or args.runner_id == "uncontrolled" or args.repeats < 5):
        parser.error("performance gates require a baseline, explicit runner-id and >= 5 repeats")
    if args.memory_gate and not hasattr(os, "wait4"):
        parser.error("memory gate requires native per-process memory measurement")
    binary = Path(args.binary).resolve()
    report = {"schema_version": 1, "runner_id": args.runner_id,
              "platform": platform.platform(), "machine": platform.machine(),
              "size": args.size, "repeats": args.repeats, "public_corpus": args.public_corpus,
              "harness_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
              "workloads": {}, "result_changes": [], "violations": []}
    output = Path(args.output).resolve()
    try:
        with tempfile.TemporaryDirectory(prefix="mori-acceptance-") as directory:
            scratch = Path(directory)
            version, _ = command([str(binary), "version"], scratch)
            report["version"] = version.stdout.strip()
            roots = generate(scratch, args.size)
            for root in roots:
                metrics = scan(binary, root, scratch, args.repeats, args.timeout)
                report["workloads"][root.name] = metrics
                report["toolchain"] = {key: metrics["sessions"][0]["tool"][key]
                                       for key in ("go_version", "goos", "goarch")}
                if root.name != "malformed":
                    assert metrics["counts"]["fragments"] == args.size, "lost synthetic fragments"
                    assert 0 <= metrics["counts"]["candidate_pairs"] <= args.size * (args.size - 1) // 2, "candidate accounting exceeds all pairs"
                    if root.name == "repeated":
                        assert metrics["counts"]["location_pairs"] == args.size * (args.size - 1) // 2, "lost identical-pair occurrences"
                else:
                    assert metrics["counts"]["parse_diagnostics"] > 0, "parse failures hidden"
            # A dense input must respect the configured work limit visibly.
            result, _ = command([str(binary), "scan", "--no-config", "--min-tokens", "12",
                                 "--max-pairs", "1", str(roots[0])], scratch, args.timeout)
            assert result.returncode != 0 and "pair" in (result.stdout + result.stderr).lower(), "pair limit ignored"
            report["cancellation"] = cancellation(binary, scratch)
            if args.public_corpus:
                manifest = json.loads(Path(__file__).with_name("acceptance-corpus.json").read_text())
                golden = json.loads(Path(__file__).with_name("acceptance-public-golden.json").read_text())
                for repo in manifest["repositories"]:
                    checkout = scratch / repo["name"]
                    checkout.mkdir()
                    for cmd in (["git", "init", "-q"], ["git", "remote", "add", "origin", repo["url"]],
                                ["git", "fetch", "--depth=1", "origin", repo["revision"]],
                                ["git", "checkout", "--detach", "FETCH_HEAD"]):
                        result, _ = command(cmd, checkout, args.timeout)
                        assert result.returncode == 0, f"corpus fetch failed: {repo['name']}: {result.stderr}"
                    metrics = scan(binary, checkout / repo["root"], scratch, args.repeats, args.timeout,
                                   ["--fragment-selection", "production"])
                    metrics["corpus_revision"] = repo["revision"]
                    assert metrics["counts"]["fragments"] > 0, "empty public corpus"
                    languages = {item["language"] for item in metrics["sessions"][0]["languages"] if item["fragments"] > 0}
                    assert set(repo.get("required_languages", [])).issubset(languages), "required corpus languages missing"
                    report["workloads"][repo["name"]] = metrics
                    observed = {"corpus_revision": repo["revision"], "counts": metrics["counts"],
                                "identity_sha256": hashlib.sha256(json.dumps(metrics["identities"], sort_keys=True,
                                                                            separators=(",", ":")).encode()).hexdigest()}
                    expected = golden["workloads"].get(repo["name"], {"unreviewed": True})
                    if observed != expected:
                        report["result_changes"].append({"workload": repo["name"], "field": "public_golden",
                                                         "before": expected, "after": observed})
            if args.baseline:
                baseline = json.loads(args.baseline.read_text())
                keys = ("runner_id", "platform", "machine", "size", "repeats", "public_corpus", "harness_sha256", "toolchain")
                assert all(baseline[key] == report[key] for key in keys), "incomparable benchmark environments"
                assert baseline["workloads"].keys() == report["workloads"].keys(), "different workload sets"
                for name, metrics in report["workloads"].items():
                    prior = baseline["workloads"][name]
                    assert prior.get("corpus_revision") == metrics.get("corpus_revision"), "different corpus revision"
                    for field in ("identities", "counts"):
                        if prior[field] != metrics[field]:
                            report["result_changes"].append({"workload": name, "field": field,
                                                             "before": prior[field], "after": metrics[field]})
                    ratio = metrics["median_ms"] / prior["median_ms"]
                    metrics["baseline_ratio"] = ratio
                    if args.timing_gate and ratio > 1 + args.max_regression:
                        report["violations"].append(f"{name}: time increased {(ratio - 1) * 100:.1f}%")
                    if all(value is not None for value in metrics["peak_rss_bytes"] + prior["peak_rss_bytes"]):
                        memory_ratio = statistics.median(metrics["peak_rss_bytes"]) / statistics.median(prior["peak_rss_bytes"])
                        metrics["baseline_memory_ratio"] = memory_ratio
                        if args.memory_gate and memory_ratio > 1 + args.max_memory_regression:
                            report["violations"].append(f"{name}: memory increased {(memory_ratio - 1) * 100:.1f}%")
                    elif args.memory_gate:
                        report["violations"].append(f"{name}: memory measurements unavailable for gate")
            if report["result_changes"] and not args.allow_result_changes:
                report["violations"].append("finding or count changes require review; see result_changes")
    except (AssertionError, OSError, ValueError, KeyError, TypeError, subprocess.TimeoutExpired) as error:
        report["violations"].append(str(error))
    finally:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, indent=2) + "\n")
    print(f"Acceptance results: {output}")
    for violation in report["violations"]:
        print(violation)
    return bool(report["violations"])


if __name__ == "__main__":
    raise SystemExit(main())
