#!/usr/bin/env python3
"""Guard patch-image workflow behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/patch-image.yaml")


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"patch-image workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def job_body(text: str, job: str) -> str:
    lines = text.splitlines()
    start = next(
        (idx for idx, line in enumerate(lines) if line == f"  {job}:"),
        None,
    )
    require(start is not None, f"missing {job} job")

    end = len(lines)
    for idx in range(start + 1, len(lines)):
        line = lines[idx]
        if line.startswith("  ") and not line.startswith("    ") and line.endswith(":"):
            end = idx
            break
    return "\n".join(lines[start:end])


def main() -> None:
    text = WORKFLOW.read_text(encoding="utf-8")
    uncommented = "\n".join(
        line for line in text.splitlines() if not line.lstrip().startswith("#")
    )

    require(
        "docker/login-action" not in uncommented,
        "GHCR login must use retrying docker login, not one-shot docker/login-action",
    )
    for job in ("scan", "patch", "finalize"):
        body = job_body(uncommented, job)
        require("- name: Login to GHCR" in body, f"{job} job must log in to GHCR")
        require(
            "bash .github/scripts/retry-docker-login.sh" in body,
            f"{job} job must use retrying GHCR login helper",
        )

    finalize = job_body(uncommented, "finalize")
    require(
        ".github/scripts/retry-docker-login.sh" in finalize.split("- name: Install mise", 1)[0],
        "finalize sparse checkout must include retry-docker-login.sh",
    )
    require(
        'LOGIN_OUTCOME: ${{ steps.ghcr-login.outcome }}' in uncommented,
        "finalize metrics must record failure when manifest/publish steps are skipped",
    )
    require(
        'PREFLIGHT_OUTCOME: ${{ steps.preflight-manifest.outcome }}' in uncommented,
        "finalize metrics must include late publish/report step outcomes",
    )
    require(
        'if [ "$outcome" = "failure" ]; then' in uncommented,
        "metrics JSON must derive failure from prior finalize step outcomes",
    )
    require(
        '--arg conclusion "${FINALIZE_CONCLUSION}"' in uncommented,
        "metrics JSON must use resolved finalize conclusion",
    )


if __name__ == "__main__":
    main()
