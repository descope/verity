#!/usr/bin/env python3
"""Guard patch-image workflow behavior that actionlint cannot express."""

from pathlib import Path
import sys


WORKFLOW = Path(".github/workflows/patch-image.yaml")
METRICS_WORKFLOW = Path(".github/workflows/metrics-finalize.yaml")
CI_WORKFLOW = Path(".github/workflows/ci.yaml")


def self_test() -> None:
    require(
        "validate-metrics-json_test.sh" not in uncomment_yaml(
            "# bash .github/scripts/validate-metrics-json_test.sh"
        ),
        "commented workflow commands must not satisfy guards",
    )
    require(
        "validate-metrics-json_test.sh" not in uncomment_yaml(
            "run: # bash .github/scripts/validate-metrics-json_test.sh"
        ),
        "inline YAML comments must not satisfy guards",
    )
    require(
        named_step_body("steps:\n", "Upload metrics artifact") is None,
        "missing workflow steps must return a controlled result",
    )
    bounded_step = named_step_body(
        """      - name: Upload metrics artifact
        with:
          retention-days: 1
      - name: Later step
        with:
          retention-days: 7
""",
        "Upload metrics artifact",
    )
    require(
        bounded_step is not None and "retention-days: 7" not in bounded_step,
        "step lookup must stop at the next sibling step",
    )
    archive = """  archive-metrics:
    secrets:
      archive-token: ${{ secrets.GITHUB_TOKEN }}
"""
    require(
        has_only_archive_token(archive),
        "the archive job's sole token mapping must be accepted",
    )
    require(
        not has_only_archive_token(
            archive + "      extra-token: ${{ secrets.EXTRA_TOKEN }}\n"
        ),
        "additional archive secrets must be rejected",
    )
    for top_level in (
        "permissions:\n  actions: write\n",
        "permissions: write-all\n",
        "permissions: read-all\n",
    ):
        require(
            top_level_grants_actions(top_level),
            "top-level Actions permission forms must be detected",
        )


def require(condition: bool, message: str) -> None:
    if not condition:
        print(f"patch-image workflow check failed: {message}", file=sys.stderr)
        sys.exit(1)


def uncomment_yaml(text: str) -> str:
    lines: list[str] = []
    for line in text.splitlines():
        single_quoted = False
        double_quoted = False
        comment_at: int | None = None
        for index, character in enumerate(line):
            if character == "'" and not double_quoted:
                single_quoted = not single_quoted
            elif character == '"' and not single_quoted:
                double_quoted = not double_quoted
            elif (
                character == "#"
                and not single_quoted
                and not double_quoted
                and (index == 0 or line[index - 1].isspace())
            ):
                comment_at = index
                break
        active = line if comment_at is None else line[:comment_at]
        if active.strip():
            lines.append(active.rstrip())
    return "\n".join(lines)


def named_step_body(text: str, step_name: str) -> str | None:
    lines = text.splitlines()
    start = next(
        (
            index
            for index, line in enumerate(lines)
            if line.lstrip() == f"- name: {step_name}"
        ),
        None,
    )
    if start is None:
        return None
    indent = len(lines[start]) - len(lines[start].lstrip())
    end = len(lines)
    for index in range(start + 1, len(lines)):
        line = lines[index]
        if (
            len(line) - len(line.lstrip()) == indent
            and line.lstrip().startswith("- name:")
        ):
            end = index
            break
    return "\n".join(lines[start:end])


def has_only_archive_token(job: str) -> bool:
    secret_lines = [
        line.strip() for line in job.splitlines() if "${{ secrets." in line
    ]
    return secret_lines == [
        "archive-token: ${{ secrets.GITHUB_TOKEN }}"
    ] and "secrets: inherit" not in job


def top_level_grants_actions(text: str) -> bool:
    for line in uncomment_yaml(text).splitlines():
        stripped = line.strip()
        if "actions:" in stripped or stripped in {
            "permissions: read-all",
            "permissions: write-all",
        }:
            return True
    return False


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
    self_test()
    text = WORKFLOW.read_text(encoding="utf-8")
    metrics_text = METRICS_WORKFLOW.read_text(encoding="utf-8")
    ci_text = CI_WORKFLOW.read_text(encoding="utf-8")
    uncommented = uncomment_yaml(text)
    metrics_uncommented = uncomment_yaml(metrics_text)
    ci_uncommented = uncomment_yaml(ci_text)

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
        has_only_archive_token(archive),
        "metrics finalization must receive only the repository token",
    )
    top_level = uncommented.split("jobs:", 1)[0]
    require(
        not top_level_grants_actions(top_level)
        and "permissions:\n      contents: write\n      actions: read" in archive,
        "artifact read access must be scoped to the archive job",
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
    finalize_upload = named_step_body(finalize, "Upload metrics artifact")
    failure_upload = named_step_body(failure_metrics, "Upload metrics artifact")
    require(
        finalize_upload is not None
        and "retention-days: 7" in finalize_upload
        and failure_upload is not None
        and "retention-days: 7" in failure_upload,
        "metrics artifacts must remain recoverable for seven days",
    )
    require(
        "bash .github/scripts/validate-metrics-json_test.sh" in ci_uncommented,
        "CI must run the metrics JSON validator regression checks",
    )
    require(
        uncommented.count("--jq '.run_started_at'") == 2
        and "--jq '.run_started_at' 2>/dev/null" not in uncommented,
        "metrics producers must fail instead of recording an empty start timestamp",
    )


if __name__ == "__main__":
    main()
