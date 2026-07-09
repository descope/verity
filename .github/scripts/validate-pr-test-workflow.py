#!/usr/bin/env python3
"""Guard PR Test scoping behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/pr-test.yaml")


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"pr-test workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def main() -> None:
    workflow = WORKFLOW.read_text(encoding="utf-8")

    require(
        "^cmd/(ci|integer|nightly|discover|scan).*\\.go$" in workflow,
        "Integer scope must include cmd/ci*.go and cmd/nightly*.go because those commands feed Integer PR jobs",
    )
    require(
        "^cmd/(ci|nightly|patch|discover|scan).*\\.go$" in workflow,
        "Copa scope must include cmd/ci*.go and cmd/nightly*.go because those commands feed Copa PR jobs",
    )
    require(
        "pr-test-result:" in workflow and "name: PR Test" in workflow,
        "PR Test must expose a stable aggregate gate for branch protection",
    )


if __name__ == "__main__":
    main()
