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

### 3. Docker Images in Workflows

Custom managers track the BuildKit references that carry the required
metadata:

- The mirrored `buildx-stable-1` digest consumed by `orchestrator.yaml` /
  `patch-image.yaml` — picked up by the `customManagers` regex rule which
  matches lines with a preceding `# renovate: datasource=docker depName=...`
  comment and a `...:TAG@sha256:...` pin.
- The `moby/buildkit:v0.29.0` image in `docker-compose.yaml` — picked up
  automatically by Renovate's built-in `docker-compose` manager.

Not currently tracked:

- The `--driver-opt image=moby/buildkit:…` pin in `pr-test.yaml`. It lives
  inside a shell command, has no digest, and has no `# renovate:` marker, so
  the regex custom manager doesn't match it. Bump it by hand when needed, or
  add a `# renovate:`-annotated digest pin if you want it auto-managed.

### 4. Tool Versions (mise.toml)

Custom manager tracks:

- Go version
- golangci-lint version

## Scheduling

- Runs before 4am UTC on Mondays
- Security updates run immediately
- Max 3 concurrent PRs to avoid overwhelming CI

## Auto-merge

✅ Auto-merged:

- Go minor/patch updates
- GitHub Actions patch updates
- Security vulnerability fixes

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

Dry-run:

```bash
LOG_LEVEL=debug renovate --dry-run --platform=github your-org/verity
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
