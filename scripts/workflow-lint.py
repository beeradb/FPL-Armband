#!/usr/bin/env python3
"""Refuse a workflow GitHub cannot parse, and a floating action tag.

# Why this exists

`.github/workflows/snapshot.yml` carried three steps with TWO `if:` keys each.
PyYAML resolves a duplicate mapping key by keeping the last one and saying
nothing; GitHub Actions rejects the whole file. So the workflow was registered
but never ran: every push produced a run with ZERO jobs and conclusion
`failure`, and `gh run view --log-failed` answered "log not found" because
there were no logs to fetch.

⚠️ **Nothing in CI could see it.** The workflow that was broken is the one that
publishes the accuracy snapshot, so its own failure was the thing that would
have reported the failure. It stayed broken from the commit that introduced the
duplicate until 2026-08-28, and was found only because someone read the run
list by hand and noticed GitHub was listing the workflow by PATH rather than by
its `name:` -- which is the visible symptom of a file GitHub never parsed.

# What it checks

1. **Duplicate mapping keys**, anywhere in any workflow. This is the error
   class above. A plain `yaml.safe_load` cannot detect it by construction, so
   the loader below reports duplicates instead of silently collapsing them.

2. **Floating action references** (`@v5`, `@main`). Every workflow here holds a
   token, and `image.yml` says it directly: "This job holds packages:write, so
   a mutable tag here is a supply-chain hole." dependabot.yml is configured to
   keep SHA pins current, so a floating tag is also a ref dependabot will not
   bump. The convention is `@<40-hex-sha> # <version>`; this enforces it.

Run: python3 scripts/workflow-lint.py
     python3 scripts/workflow-lint.py --selftest   (prove both checks can fail)
"""
import pathlib
import re
import sys

import yaml

WORKFLOWS = pathlib.Path(".github/workflows")
SHA = re.compile(r"^[0-9a-f]{40}$")
# Local (`./.github/...`) and reusable-workflow refs are not registry actions.
LOCAL = ("./", ".\\")


class DupKeyLoader(yaml.SafeLoader):
    """A loader that records duplicate keys rather than collapsing them."""


def _no_duplicates(loader, node, deep=False):
    seen, out = {}, {}
    for k_node, v_node in node.value:
        key = loader.construct_object(k_node, deep=deep)
        if key in seen:
            loader.duplicates.append((key, seen[key] + 1, k_node.start_mark.line + 1))
        seen[key] = k_node.start_mark.line
        out[key] = loader.construct_object(v_node, deep=deep)
    return out


DupKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _no_duplicates
)


def check(path):
    problems = []
    text = path.read_text()

    loader = DupKeyLoader(text)
    loader.duplicates = []
    try:
        loader.get_single_data()
    except yaml.YAMLError as e:
        return [f"{path}: does not parse as YAML at all: {e}"]
    finally:
        dups = loader.duplicates
        loader.dispose()

    for key, first, second in dups:
        problems.append(
            f"{path}:{second}: duplicate key {key!r} (first set on line {first}). "
            f"GitHub rejects the whole file; PyYAML keeps only the last one. "
            f"If both conditions are wanted, join them with `&&`."
        )

    for i, line in enumerate(text.splitlines(), 1):
        m = re.search(r"uses:\s*(\S+)", line)
        if not m:
            continue
        ref = m.group(1)
        if ref.startswith(LOCAL) or "@" not in ref:
            continue
        version = ref.rsplit("@", 1)[1]
        if not SHA.match(version):
            problems.append(
                f"{path}:{i}: `{ref}` is a floating tag. Pin it as "
                f"`<action>@<40-hex-sha> # {version}` -- these workflows hold "
                f"tokens, and dependabot only bumps pinned refs."
            )
    return problems


SELFTEST_YAML = """\
name: selftest
on:
  push:
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - name: two ifs
        if: github.event_name != 'pull_request'
        if: vars.SOMETHING == 'true'
        run: 'true'
"""


def selftest():
    """Prove both checks can fail, on a file that carries both defects.

    Follows this repo's `--selftest` idiom: a checker nobody has watched fail
    is a checker nobody knows works. The defects below are the two this script
    exists for -- the duplicate `if:` that silently disabled snapshot.yml, and
    a floating action tag.
    """
    import tempfile

    with tempfile.TemporaryDirectory() as d:
        f = pathlib.Path(d) / "selftest.yml"
        f.write_text(SELFTEST_YAML)
        problems = check(f)

    dup = [p for p in problems if "duplicate key" in p]
    floating = [p for p in problems if "floating tag" in p]
    if not dup:
        print("SELFTEST FAILED: the duplicate-key check did not fire", file=sys.stderr)
        return 1
    if not floating:
        print("SELFTEST FAILED: the floating-tag check did not fire", file=sys.stderr)
        return 1
    print("selftest: both checks fired on a file carrying both defects.")
    return 0


def main():
    if "--selftest" in sys.argv[1:]:
        return selftest()
    if not WORKFLOWS.is_dir():
        print(f"no {WORKFLOWS}/ -- nothing to check", file=sys.stderr)
        return 1
    files = sorted(
        p for p in WORKFLOWS.iterdir() if p.suffix in (".yml", ".yaml")
    )
    if not files:
        print(f"no workflow files under {WORKFLOWS}/", file=sys.stderr)
        return 1

    problems = [p for f in files for p in check(f)]
    for p in problems:
        print(p, file=sys.stderr)
    if problems:
        print(
            f"\n{len(problems)} problem(s) across {len(files)} workflow file(s).",
            file=sys.stderr,
        )
        return 1
    print(f"{len(files)} workflow files: parse clean, no duplicate keys, every action pinned.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
