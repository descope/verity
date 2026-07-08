#!/usr/bin/env python3
"""Guard nightly orchestration behavior that actionlint cannot express."""

from pathlib import Path
import sys


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"nightly workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def uncomment(path: str) -> str:
    return "\n".join(
        line
        for line in Path(path).read_text(encoding="utf-8").splitlines()
        if not line.lstrip().startswith("#")
    )


def main() -> None:
    orchestrator = uncomment(".github/workflows/orchestrator.yaml")
    chart_gen = uncomment(".github/workflows/chart-gen.yaml")
    build_site = uncomment(".github/workflows/build-site.yaml")
    chart_integration = uncomment(".github/workflows/chart-integration.yaml")
    ci = uncomment(".github/workflows/ci.yaml")

    require(
        "for attempt in 1 2 3 4 5; do" in orchestrator
        and "gh workflow run patch-image.yaml" in orchestrator
        and 'if [ "$dispatched" -ne "$expected_count" ]; then' in orchestrator,
        "Copa orchestrator dispatch must retry and verify all patch runs dispatched",
    )
    require(
        "bash .github/scripts/wait-for-workflows.sh patch-image.yaml" in chart_gen
        and "actions: read" in chart_gen,
        "chart generation must wait for active patch-image producer runs",
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
