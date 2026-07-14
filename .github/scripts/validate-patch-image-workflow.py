#!/usr/bin/env python3
"""Guard patch-image workflow behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/patch-image.yaml")
METRICS_WORKFLOW = Path(".github/workflows/metrics-finalize.yaml")
CI_WORKFLOW = Path(".github/workflows/ci.yaml")


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
    metrics_text = METRICS_WORKFLOW.read_text(encoding="utf-8")
    ci_text = CI_WORKFLOW.read_text(encoding="utf-8")
    uncommented = "\n".join(
        line for line in text.splitlines() if not line.lstrip().startswith("#")
    )
    metrics_uncommented = "\n".join(
        line
        for line in metrics_text.splitlines()
        if not line.lstrip().startswith("#")
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
    require(
        'if [ -z "$TARGET_REF" ] || [ -z "$MANIFEST_DIGEST" ]; then'
        in uncommented,
        "successful metrics must require published target and manifest outputs",
    )
    require(
        "- name: Resolve target manifest" in finalize
        and "id: target" in finalize
        and 'TARGET_REF: ${{ steps.target.outputs.final-tag }}' in finalize
        and 'MANIFEST_DIGEST: ${{ steps.target.outputs.digest }}' in finalize,
        "metrics must resolve the target even when the publish step is a no-op",
    )

    archive = job_body(uncommented, "archive-metrics")
    require(
        "uses: ./.github/workflows/metrics-finalize.yaml" in archive,
        "patch-image must call the metrics finalizer directly",
    )
    require(
        'archive-token: ${{ secrets.GITHUB_TOKEN }}' in archive
        and "secrets: inherit" not in archive,
        "metrics finalization must receive only the repository token",
    )
    require(
        "inputs.is-pr != true" in archive,
        "PR patch runs must not write to the metrics archive",
    )
    require(
        "workflow_call:" in metrics_uncommented
        and "workflow_run:" not in metrics_uncommented,
        "metrics finalization must be reusable instead of relying on workflow_run",
    )
    require(
        "bash .github/scripts/validate-metrics-json.sh" in metrics_uncommented,
        "metrics artifacts must be schema-validated before commit",
    )
    require(
        "No metrics artifacts found" in metrics_uncommented,
        "missing metrics artifacts must fail finalization",
    )
    failure_metrics = job_body(uncommented, "metrics-on-failure")
    require(
        "retention-days: 7" in finalize.rsplit("- name: Upload metrics artifact", 1)[1]
        and "retention-days: 7"
        in failure_metrics.rsplit("- name: Upload metrics artifact", 1)[1],
        "metrics artifacts must remain recoverable for seven days",
    )
    require(
        "bash .github/scripts/validate-metrics-json_test.sh" in ci_text,
        "CI must run the metrics JSON validator regression checks",
    )


if __name__ == "__main__":
    main()
