from pathlib import Path

workflow = Path(".github/workflows/ci.yaml").read_text()

required = [
    "^site/|^\\.github/workflows/ci\\.yaml$|^mise\\.toml$",
    "python3 .github/scripts/validate-ci-workflow.py",
    "python3 .github/scripts/validate-nightly-workflows.py",
    "python3 .github/scripts/validate-pr-test-workflow.py",
]

missing = [needle for needle in required if needle not in workflow]
if missing:
    raise SystemExit(f"CI workflow guard missing: {missing}")

if "pull-requests: read" in workflow:
    raise SystemExit("CI workflow grants unused pull-requests permission")
