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
        workflow.count('["amd64", "arm64"][] as $arch') == 2
        and workflow.count("runs-on: ${{ matrix.runner }}") == 2
        and workflow.count("ubuntu-24.04-arm") >= 2,
        "Integer build and smoke matrices must place each architecture on its native runner",
    )
    require(
        "for package_arch in x86_64 aarch64; do" not in workflow
        and workflow.count('--arch "$INTEGER_PACKAGE_ARCH"') >= 3
        and workflow.count("for arch in amd64 arm64; do") == 1,
        "Every Integer matrix leg must build only its native package and image architecture",
    )
    require(
        workflow.count('docker load --input "$tar_path"') == 2
        and workflow.count('docker image inspect "$loaded_ref"') == 2
        and workflow.count("docker load did not report an image reference") == 2
        and workflow.count("runtime architecture mismatch") == 2,
        "Every Integer build and smoke leg must verify the loaded runtime architecture",
    )
    require(
        "docker/setup-qemu-action" not in workflow,
        "Integer PR jobs must not compile aarch64 packages through QEMU",
    )
    require(
        workflow.count('--fail-on-severity "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"') == 2,
        "Every Integer image path must retain strict Trivy exit-code enforcement for every severity",
    )
    require(
        workflow.count("name: Cache Trivy database") == 2
        and workflow.count("path: ~/.cache/trivy") == 2
        and workflow.count(
            "key: trivy-db-${{ steps.trivy-cache-key.outputs.version }}-${{ steps.trivy-cache-key.outputs.date }}"
        )
        == 2
        and workflow.count(
            "restore-keys: trivy-db-${{ steps.trivy-cache-key.outputs.version }}-"
        )
        == 2,
        "Integer build and smoke jobs must cache the Trivy database by pinned version and UTC date",
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
        'smoke_count=$(integer_matrix_entry_count "$EXPECTED_INTEGER_SMOKE_MATRIX" smoke)'
        in workflow
        and "needs.detect-changed-images.outputs.smoke-has-changes == 'true'"
        in workflow
        and 'require_result "integer-smoke-test" "$INTEGER_SMOKE_RESULT" skipped'
        in workflow
        and "expected Integer build matrix must not be empty when changes are present"
        in workflow,
        "The required aggregate must distinguish empty smoke-only coverage from mandatory strict builds",
    )
    require(
        '[ "$image" = linkerd ] && [ "$version" = 25 ] && [ "$type" = default ]' in workflow
        and "--staged" in workflow
        and '--arch "$INTEGER_PACKAGE_ARCH"' in workflow,
        "The Linkerd pinning canary must retain staged package coverage on each native architecture",
    )
    require(
        workflow.count("group_by((.key / 16 | floor))") == 2
        and workflow.count('["amd64", "arm64"][] as $arch') == 2
        and "expected-matrix" in workflow
        and "expected-smoke-matrix" in workflow,
        "Affected Integer entries must remain bounded to 32 native architecture batches without losing aggregate expectations",
    )


if __name__ == "__main__":
    main()
