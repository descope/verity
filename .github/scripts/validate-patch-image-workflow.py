#!/usr/bin/env python3
"""Guard patch-image workflow behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/patch-image.yaml")


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"patch-image workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def main() -> None:
    text = WORKFLOW.read_text(encoding="utf-8")

    require(
        "docker/login-action" not in text,
        "GHCR login must use retrying docker login, not one-shot docker/login-action",
    )
    require(
        text.count("bash .github/scripts/retry-docker-login.sh") >= 3,
        "scan, patch, and finalize jobs must retry GHCR login",
    )
    require(
        'LOGIN_OUTCOME: ${{ steps.ghcr-login.outcome }}' in text,
        "finalize metrics must record failure when manifest/publish steps are skipped",
    )
    require(
        'if [ "$outcome" = "failure" ]; then' in text,
        "metrics JSON must derive failure from prior finalize step outcomes",
    )
    require(
        '--arg conclusion "${FINALIZE_CONCLUSION}"' in text,
        "metrics JSON must use resolved finalize conclusion",
    )


if __name__ == "__main__":
    main()
