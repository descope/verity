from pathlib import Path

workflow = Path(".github/workflows/chart-integration.yaml").read_text()

required = [
    "image_re=\"$(printf '%s' \"$image\" | sed 's/[][(){}.^$?+*|\\\\/]/\\\\&/g')\"",
    "${image_re}([:@'\\\"[:space:]]|$)|\\b${image_re}\\b",
]

missing = [needle for needle in required if needle not in workflow]
if missing:
    raise SystemExit(f"chart-integration workflow guard missing: {missing}")

if "${image}([:@'\\\"[:space:]]|$)|\\b${image}\\b" in workflow:
    raise SystemExit("chart-integration workflow uses an unescaped image regex")
