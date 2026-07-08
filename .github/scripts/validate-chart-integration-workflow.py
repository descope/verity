from pathlib import Path

workflow = Path(".github/workflows/chart-integration.yaml").read_text()

required = [
    "./verity ci plan \\",
    "--kind chart \\",
    "json=\"$(jq -c '[.matrix.include[].chart]' ci-plan.json)\"",
]

missing = [needle for needle in required if needle not in workflow]
if missing:
    raise SystemExit(f"chart-integration workflow guard missing: {missing}")

if "grep -Eq \"verity-org/" in workflow:
    raise SystemExit("chart-integration workflow must not rebuild chart/image matching in shell")
