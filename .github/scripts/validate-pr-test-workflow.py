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
        "^internal/(ci|integer)/" in workflow,
        "Integer scope must include internal/ci because it selects affected build and smoke variants",
    )
    require(
        "^cmd/(ci|nightly|patch|discover|scan).*\\.go$" in workflow,
        "Copa scope must include cmd/ci*.go and cmd/nightly*.go because those commands feed Copa PR jobs",
    )
    require(
        "pr-test-result:" in workflow and "name: PR Test" in workflow,
        "PR Test must expose a stable aggregate gate for branch protection",
    )
    require(
        '["amd64", "arm64"][] as $arch' not in workflow,
        "Integer dual-architecture coverage must not multiply matrices beyond GitHub's 256-job limit",
    )
    require(
        workflow.count("for package_arch in x86_64 aarch64; do") == 2
        and workflow.count('--arch "$package_arch"') == 2
        and workflow.count("for arch in amd64 arm64; do") >= 4,
        "Every Integer build and smoke leg must build its architecture-specific package and image",
    )
    require(
        workflow.count('docker load --input "$tar_path"') == 2
        and workflow.count('docker image inspect "$loaded_ref"') == 2
        and workflow.count("docker load did not report an image reference") == 2
        and workflow.count("runtime architecture mismatch") == 2,
        "Every Integer build and smoke leg must verify the loaded runtime architecture",
    )
    require(
        workflow.count("name: Set up QEMU for dual-architecture verification") == 2,
        "Both Integer jobs must register arm64 binfmt before aarch64 package builds",
    )
    require(
        workflow.count('--fail-on-severity "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"') == 2,
        "Every Integer image path must retain strict Trivy exit-code enforcement for every severity",
    )
    require(
        "--exit-code 0" not in workflow
        and workflow.count("--exit-code 1") >= 3,
        "Integer Trivy scans must fail closed and never run report-only",
    )
    require(
        "EXPECTED_INTEGER_MATRIX: ${{ needs.detect-changed-images.outputs.expected-matrix }}" in workflow
        and "EXPECTED_INTEGER_SMOKE_MATRIX: ${{ needs.detect-changed-images.outputs.expected-smoke-matrix }}" in workflow
        and "for arch in amd64 arm64; do" in workflow
        and "missing successful Integer ${kind} security leg" in workflow,
        "The required aggregate must reject any absent, skipped, cancelled, or failed architecture leg",
    )
    require(
        '[ "$image" = linkerd ] && [ "$version" = 25 ] && [ "$type" = default ]' in workflow
        and "matrix.arch" not in workflow,
        "The Linkerd dual-architecture pinning canary must run once without standing in for the global arm64 gate",
    )
    require(
        workflow.count("group_by((.key / 16 | floor))") == 2
        and "expected-matrix" in workflow
        and "expected-smoke-matrix" in workflow,
        "Affected Integer entries must be batched below GitHub's 256-job matrix limit without losing aggregate expectations",
    )


if __name__ == "__main__":
    main()
