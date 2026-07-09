#!/usr/bin/env python3
"""Guard nightly orchestration behavior that actionlint cannot express."""

from pathlib import Path
import re
import sys


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"nightly workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def uncomment_text(text: str) -> str:
    lines: list[str] = []
    block_parent_indent: int | None = None

    for line in text.splitlines():
        stripped = line.lstrip()
        indent = len(line) - len(stripped)

        if block_parent_indent is not None:
            if stripped and indent <= block_parent_indent:
                block_parent_indent = None
            else:
                lines.append(line)
                continue

        if stripped.startswith("#"):
            continue

        lines.append(line)
        if re.search(r"(:|-\s*)\s*[|>][+-]?\s*(?:#.*)?$", line):
            block_parent_indent = indent

    return "\n".join(lines)


def uncomment(path: str) -> str:
    return uncomment_text(Path(path).read_text(encoding="utf-8"))


def self_test() -> None:
    sample = """
name: example
# YAML comment
jobs:
  test:
    steps:
      - run: |
          # shell comments inside block scalars must be preserved
          #!/usr/bin/env bash
          echo ok
    # another YAML comment
permissions: {}
"""
    got = uncomment_text(sample)
    require("# YAML comment" not in got, "YAML comments should be stripped")
    require(
        "#!/usr/bin/env bash" in got and "# shell comments inside block scalars" in got,
        "block-scalar shell comments should be preserved",
    )
    require("# another YAML comment" not in got, "YAML comments after block scalars should be stripped")


def main() -> None:
    self_test()
    orchestrator = uncomment(".github/workflows/orchestrator.yaml")
    integer_orchestrator = uncomment(".github/workflows/integer-orchestrator.yaml")
    chart_gen = uncomment(".github/workflows/chart-gen.yaml")
    build_site = uncomment(".github/workflows/build-site.yaml")
    chart_integration = uncomment(".github/workflows/chart-integration.yaml")
    ci = uncomment(".github/workflows/ci.yaml")
    wait_helper = Path(".github/scripts/wait-for-workflows.sh").read_text(encoding="utf-8")

    require(
        "nightly plan" in orchestrator
        and "--family copa" in orchestrator
        and "nightly dispatch" in orchestrator
        and "--family copa" in orchestrator
        and "gh workflow run patch-image.yaml" not in orchestrator,
        "Copa orchestrator must plan dirty targets and dispatch through verity nightly, not shell gh loops",
    )
    require(
        "nightly plan" in integer_orchestrator
        and "--family integer" in integer_orchestrator
        and "nightly dispatch" in integer_orchestrator
        and "--family integer" in integer_orchestrator
        and "gh workflow run integer-build-image.yaml" not in integer_orchestrator,
        "Integer orchestrator must scan published targets and dispatch through verity nightly, not shell gh loops",
    )
    require(
        'IMAGES: ${{ needs.discover.outputs.images }}' not in integer_orchestrator
        and "__IMAGES_JSON_EOF__" in integer_orchestrator,
        "Integer orchestrator must not pass the large dispatch matrix through an environment variable",
    )
    require(
        "bash .github/scripts/wait-for-workflows.sh patch-image.yaml" in chart_gen
        and "actions: read" in chart_gen,
        "chart generation must wait for active patch-image producer runs",
    )
    require(
        "--method GET" in wait_helper
        and "--paginate" in wait_helper
        and ".database_id" not in wait_helper,
        "wait helper must use GET pagination and REST run ids",
    )
    require(
        "bash .github/scripts/wait-for-workflows.sh patch-image.yaml integer-build-image.yaml chart-gen.yaml"
        in build_site
        and "actions: read" in build_site,
        "site build must wait for active patch, Integer, and chart generation producer runs",
    )
    require(
        "  schedule:" not in chart_integration,
        "chart-integration must not also schedule itself beside workflow_run",
    )
    require(
        "python3 .github/scripts/validate-nightly-workflows.py" in ci,
        "CI must run the nightly workflow validator",
    )


if __name__ == "__main__":
    main()
