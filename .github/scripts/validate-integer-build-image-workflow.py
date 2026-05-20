#!/usr/bin/env python3
"""Guard integer-build-image workflow retry behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/integer-build-image.yaml")
RETRY_HELPER = Path(".github/scripts/retry-registry-command.sh")


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"integer-build-image workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def uncomment(text: str) -> str:
    return "\n".join(
        line for line in text.splitlines() if not line.lstrip().startswith("#")
    )


def main() -> None:
    workflow = uncomment(WORKFLOW.read_text(encoding="utf-8"))
    helper = RETRY_HELPER.read_text(encoding="utf-8")

    require(
        "docker/login-action" not in workflow,
        "Integer Build Image must use retrying GHCR login, not one-shot docker/login-action",
    )
    require(
        "bash .github/scripts/retry-docker-login.sh" in workflow,
        "Integer Build Image must use retrying GHCR login helper",
    )

    for label in ("apko publish", "crane digest", "cosign sign"):
        require(
            "bash .github/scripts/retry-registry-command.sh" in workflow
            and f'"{label}"' in workflow,
            f"{label} must run through retry-registry-command.sh",
        )

    require(
        'digest=$(bash .github/scripts/retry-registry-command.sh \\' in workflow,
        "digest retrieval must capture retry helper stdout",
    )
    require(
        'MAX_ATTEMPTS="${REGISTRY_COMMAND_ATTEMPTS:-4}"' in helper,
        "retry helper must default to a bounded number of attempts",
    )
    require(
        "sleep \"$wait_seconds\"" in helper and "RANDOM" in helper,
        "retry helper must back off with jitter between attempts",
    )
    require(
        ">&2" in helper,
        "retry helper must log to stderr so crane digest stdout stays clean",
    )


if __name__ == "__main__":
    main()
