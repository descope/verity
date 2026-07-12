# GitHub Actions Scripts

Shell scripts used by GitHub Actions workflows. All follow `set -euo pipefail`
and are shellcheck validated.

## Patching pipeline

| Script              | Used by                                        | Purpose                                                                                                                                        |
| ------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `patch-image.sh`    | `patch-image.yaml`, `pr-test.yaml`             | `./verity patch` (Copa as a library) + crane fallback for one platform image                                                                   |
| `scan-before.sh`    | `pr-test.yaml`                                 | Pre-patch Trivy scan                                                                                                                           |
| `verify-patched.sh` | `pr-test.yaml`                                 | Verify patched image vs. pre-patch CVE state                                                                                                   |
| `push-reports.sh`   | `patch-image.yaml`, `integer-build-image.yaml` | Push JSON files (pre/post Copa scan reports, Integer build reports) to the `reports` branch via the Contents API (with retry)                  |

## Copa input lookup

| Script                  | Used by        | Purpose                                                                                                           |
| ----------------------- | -------------- | ----------------------------------------------------------------------------------------------------------------- |
| `read-catalog-entry.sh` | `pr-test.yaml` | Look up `image` (and optional `goVcsUrl` / `goVcsTagPrefix`) from `copa-config.yaml` for a given image name + tag |

## Issue-to-PR automation

| Script                      | Used by          | Purpose                                             |
| --------------------------- | ---------------- | --------------------------------------------------- |
| `parse-image-issue-form.sh` | `new-issue.yaml` | Parse image fields from the GitHub issue form body  |
| `add-standalone-image.sh`   | `new-issue.yaml` | Add image entry to `copa-config.yaml` and open a PR |

## Utility / manual

Manual verification commands live in the top-level README.

## Development

```bash
# Lint all scripts
make lint-shell
```
