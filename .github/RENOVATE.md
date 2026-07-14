# Renovate Configuration

This repository uses Renovate to automatically update dependencies and trigger the patching workflows.

## What Gets Updated

### 1. Go Dependencies (go.mod)

- Security vulnerabilities auto-merge
- Minor/patch updates auto-merge
- Major updates require manual review

### 2. GitHub Actions

- Patch updates auto-merge
- Minor/major updates require review

### 3. Docker Images in Workflows and Compose

Custom regex managers track Docker references embedded in workflow shell
blocks, including both digest pins and `docker buildx --driver-opt image=...`
tags. Renovate's built-in `docker-compose` manager is restricted to the root
`docker-compose.yaml`; `images/docker-compose.yaml` is an Integer catalog entry,
not a Compose file.

Every workflow shell reference must have a preceding
`# renovate: datasource=docker depName=...` marker. The coverage validator
rejects unannotated `--driver-opt image=...` pins.

### 4. Bespoke Integer Packages

Custom managers extract every externally maintained recipe version under
`packages/bespoke/`:

- GitHub tag-based recipes, including OpenSSL and Linkerd tag prefixes
- PostgreSQL `REL_X_Y` tags through a custom GitHub-tags datasource
- Airflow and embedded Python pins through PyPI
- embedded Go module remediation pins through the Go datasource
- HAProxy release tags

When an image's `latest: true` key is the exact version of a bespoke GitHub
recipe, the image key carries the same Renovate dependency annotation. Renovate
therefore updates the recipe and its consuming image definition on the same
branch.

Recipe updates require Dependency Dashboard approval because package bumps may
also require expected-commit, checksum, vendored dependency, epoch, or
`packages/upstream.lock.json` maintenance before they are buildable.

The two locally versioned recipes with no external release source are
deliberately excluded: `logstash-env2yaml` and
`verity-opensearch-dashboards-config`.

### 5. Integer Image Version Streams

The `versions:` keys in `images/*.yaml` are a set of Wolfi package streams, not
a single dependency value. Renovate must not replace every older stream with
the newest one. The daily `integer-sync.yaml` workflow runs
`verity integer sync --apply` against Wolfi APKINDEX and opens or updates a PR
that adds newly published streams while preserving supported older streams.
Each PR is capped at 20 changed image definitions to keep Integer build and
Trivy validation bounded; subsequent runs advance after the current batch is
merged.

### 6. Tool Versions (mise.toml)

The built-in mise manager tracks all tool pins in `mise.toml`.

## Scheduling

- Renovate runs without a repository schedule restriction
- Security updates run immediately
- Max 3 concurrent PRs to avoid overwhelming CI
- Integer package-stream discovery runs daily at 01:30 UTC

## Auto-merge

✅ Auto-merged:

- Go minor/patch updates
- GitHub Actions patch updates
- Security vulnerability fixes
- Helm chart minor/patch updates — one PR per chart (never grouped), and
  automerge is gated by the required "Chart Integration" check, which runs
  the smoke test for exactly the bumped chart

⚠️ Requires review:

- Major version updates
- Breaking changes

## Labels

PRs are automatically labeled:

- `dependencies` - All dependency updates
- `go` - Go dependency updates
- `github-actions` - GitHub Actions updates
- `security` - Security vulnerability fixes

## Enabling Renovate

### For GitHub.com repositories

1. **Install Renovate App:**
   - Visit https://github.com/apps/renovate
   - Click "Install"
   - Select this repository

2. **Or enable GitHub-native Dependency Graph:**
   - Repository Settings → Security → Dependency graph
   - Enable Dependabot alerts

### For Self-Hosted

Run Renovate as a cron job or GitHub Action:

```yaml
# .github/workflows/renovate.yaml
name: Renovate
on:
  schedule:
    - cron: '0 0 * * 1'  # Weekly on Monday
  workflow_dispatch:

jobs:
  renovate:
    runs-on: ubuntu-latest
    steps:
      - uses: renovatebot/github-action@v40
        with:
          token: ${{ secrets.RENOVATE_TOKEN }}
```

## Testing Renovate Config

Validate configuration:

```bash
# Using Renovate CLI
npm install -g renovate
renovate-config-validator .github/renovate.json

# Or use online validator
# https://app.renovatebot.com/config-validator
```

Validate effective extraction locally:

```bash
RENOVATE_PLATFORM=local \
RENOVATE_DRY_RUN=extract \
RENOVATE_REQUIRE_CONFIG=required \
renovate

python3 .github/scripts/validate-renovate-coverage.py
```

## Customization

### Change Schedule

Edit `.github/renovate.json`:

```json
{
  "schedule": ["every weekend"]
}
```

Common schedules:

- `["at any time"]` - No schedule restrictions
- `["after 6pm"]` - Only after hours
- `["every weekday"]` - Monday-Friday

### Disable Auto-merge

Remove automerge rules:

```json
{
  "packageRules": [
    {
      "matchManagers": ["gomod"],
      "matchUpdateTypes": ["minor", "patch"],
      "automerge": false
    }
  ]
}
```

## Dependency Dashboard

Renovate creates a Dependency Dashboard issue tracking:

- Pending updates
- Rate-limited PRs
- Errors encountered
- Configuration issues

Find it in Issues → Dependency Dashboard

## Troubleshooting

### Renovate not creating PRs

Check:

1. Renovate app is installed and has access
2. PR limit not reached (default: 3 concurrent)
3. Schedule allows updates now
4. Check Dependency Dashboard for errors

### PRs not auto-merging

Verify:

1. Branch protection allows auto-merge
2. CI passes successfully
3. Update matches automerge rules
4. No merge conflicts

## Related Documentation

- [Renovate Docs](https://docs.renovatebot.com/) - Full documentation
