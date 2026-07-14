# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# python3 .github/scripts/validate-renovate-coverage.py

import json
import re
from pathlib import Path
from typing import Final


ROOT: Final = Path(__file__).resolve().parents[2]
CONFIG_PATH: Final = ROOT / ".github/renovate.json"
SYNC_WORKFLOW_PATH: Final = ROOT / ".github/workflows/integer-sync.yaml"

REQUIRED_CUSTOM_MANAGERS: Final = {
    "Annotated dependencies",
    "Bespoke GitHub package releases",
    "Bespoke prefixed GitHub package releases",
    "PostgreSQL package releases",
    "Airflow package release",
    "HAProxy package releases",
    "Linkerd edge package releases",
    "Python packages embedded in bespoke recipes",
    "Go modules embedded in bespoke recipes",
    "Docker image digest pins in workflow env/run blocks",
    "Docker image tags in workflow run blocks",
}
LOCAL_RECIPE_VERSIONS: Final = {
    "packages/bespoke/logstash-env2yaml.yaml",
    "packages/bespoke/verity-opensearch-dashboards-config.yaml",
}
SPECIAL_RECIPE_SOURCES: Final = {
    "packages/bespoke/airflow-3.yaml",
    "packages/bespoke/haproxy-3.0.yaml",
    "packages/bespoke/haproxy-3.1.yaml",
    "packages/bespoke/linkerd2-cli-25.yaml",
}
SUPPORTED_GITHUB_TAGS: Final = {
    "${{package.version}}",
    "v${{package.version}}",
    "openssl-${{package.version}}",
    "REL_${{vars.mangled-package-version}}",
}


def require(errors: list[str], condition: bool, message: str) -> None:
    if not condition:
        errors.append(message)


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def validate_config(errors: list[str]) -> None:
    config = json.loads(CONFIG_PATH.read_text())
    enabled_managers = set(config.get("enabledManagers", []))
    require(errors, "custom.regex" in enabled_managers, "custom.regex is not enabled")

    descriptions = {
        manager.get("description") for manager in config.get("customManagers", [])
    }
    missing = REQUIRED_CUSTOM_MANAGERS - descriptions
    require(errors, not missing, f"missing custom managers: {sorted(missing)}")

    compose_patterns = config.get("docker-compose", {}).get("managerFilePatterns", [])
    require(
        errors,
        compose_patterns == ["/^docker-compose\\.ya?ml$/"],
        "docker-compose manager must only inspect the root compose file",
    )
    require(
        errors,
        "images/docker-compose.yaml" in config.get("ignorePaths", []),
        "image catalog's docker-compose.yaml must be ignored by built-in managers",
    )


def validate_recipes(errors: list[str]) -> None:
    pypi_pins = 0
    go_pins = 0
    for path in sorted((ROOT / "packages/bespoke").rglob("*.yaml")):
        text = path.read_text()
        if not re.search(r"^  version:\s*[^\n]+", text, re.MULTILINE):
            continue

        name = relative(path)
        repositories = re.findall(
            r"^\s+repository:\s*https://github\.com/[^\s]+", text, re.MULTILINE
        )
        tags = re.findall(r"^\s+tag:\s*(\S+)", text, re.MULTILINE)

        if repositories:
            require(errors, bool(tags), f"{name}: GitHub source has no tag")
            require(
                errors,
                all(tag in SUPPORTED_GITHUB_TAGS for tag in tags),
                f"{name}: unsupported GitHub tag shape {tags}",
            )
        else:
            require(
                errors,
                name in SPECIAL_RECIPE_SOURCES or name in LOCAL_RECIPE_VERSIONS,
                f"{name}: version has no Renovate source classification",
            )

        pypi_pins += len(
            re.findall(r"[\"'][A-Za-z0-9_.-]+==[0-9][A-Za-z0-9_.+-]*[\"']", text)
        )
        go_pins += len(
            re.findall(
                r"\b(?:github\.com|golang\.org|go\.opentelemetry\.io|"
                r"go\.mongodb\.org)/[A-Za-z0-9_./-]+@v[0-9][A-Za-z0-9_.+-]*",
                text,
            )
        )

    require(errors, pypi_pins > 0, "no embedded PyPI pins were discovered")
    require(errors, go_pins > 0, "no embedded Go module pins were discovered")


def validate_image_catalog(errors: list[str]) -> None:
    image_files = sorted((ROOT / "images").glob("*.yaml"))
    require(errors, bool(image_files), "image catalog is empty")
    for path in image_files:
        text = path.read_text()
        require(errors, "upstream:\n" in text, f"{relative(path)}: missing upstream")
        require(errors, "versions:\n" in text, f"{relative(path)}: missing versions")

        latest_match = re.search(
            r'^  ["\']?(?P<version>[^"\':]+)["\']?:\n'
            r"(?:    [^\n]*\n)*?    latest:\s*true$",
            text,
            re.MULTILINE,
        )
        recipe_match = re.search(r'\bbespoke:\s*["\']?([^"\'\s{]+)', text)
        if latest_match is None or recipe_match is None:
            continue

        recipe_path = ROOT / "packages/bespoke" / recipe_match.group(1)
        if not recipe_path.exists():
            continue
        recipe = recipe_path.read_text()
        version_match = re.search(r'^  version:\s*["\']?([^"\'\n]+)', recipe, re.MULTILINE)
        repository_match = re.search(
            r"^\s+repository:\s*https://github\.com/([^\s]+)$", recipe, re.MULTILINE
        )
        tag_match = re.search(r"^\s+tag:\s*(\S+)", recipe, re.MULTILINE)
        if (
            version_match is None
            or repository_match is None
            or tag_match is None
            or version_match.group(1) != latest_match.group("version")
            or tag_match.group(1) not in {"${{package.version}}", "v${{package.version}}"}
        ):
            continue

        repository = repository_match.group(1).removesuffix(".git")
        marker = (
            "# renovate: datasource=github-tags "
            f"depName={repository} versioning=semver-coerced"
        )
        require(
            errors,
            marker in text,
            f"{relative(path)}: source-maintained latest version lacks Renovate marker",
        )

    require(errors, SYNC_WORKFLOW_PATH.exists(), "integer sync workflow is missing")
    if SYNC_WORKFLOW_PATH.exists():
        workflow = SYNC_WORKFLOW_PATH.read_text()
        for needle in (
            "./verity integer sync --apply",
            "contents: write",
            "pull-requests: write",
            "gh pr create",
        ):
            require(errors, needle in workflow, f"integer sync workflow missing {needle}")


def validate_workflow_images(errors: list[str]) -> None:
    image_pattern = re.compile(r"--driver-opt image=([A-Za-z0-9._/-]+):[^\\\s]+")
    for path in sorted((ROOT / ".github/workflows").glob("*.yaml")):
        lines = path.read_text().splitlines()
        for index, line in enumerate(lines):
            match = image_pattern.search(line)
            if match is None:
                continue
            marker = f"# renovate: datasource=docker depName={match.group(1)}"
            preceding = lines[max(0, index - 8) : index]
            require(
                errors,
                any(marker in candidate for candidate in preceding),
                f"{relative(path)}:{index + 1}: image pin lacks {marker}",
            )


def main() -> int:
    errors: list[str] = []
    validate_config(errors)
    validate_recipes(errors)
    validate_image_catalog(errors)
    validate_workflow_images(errors)
    if errors:
        raise SystemExit("Renovate coverage validation failed:\n- " + "\n- ".join(errors))
    print("Renovate coverage validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
