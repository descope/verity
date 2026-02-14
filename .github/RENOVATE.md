# Renovate Configuration

This repository uses Renovate to automatically update dependencies and trigger the patching workflows.

## What Gets Updated

### 1. Helm Charts (Chart.yaml)
```yaml
dependencies:
  - name: prometheus
    version: "25.8.0"  # ← Renovate updates this
```

**When updated:**
- Renovate creates PR with version bump
- `patch-on-pr.yaml` workflow triggers automatically
- Patched wrapper charts committed to the same PR
- PR is complete with patched images, ready to merge

### 2. Go Dependencies (go.mod)
- Security vulnerabilities auto-merge
- Minor/patch updates auto-merge
- Major updates require manual review

### 3. GitHub Actions
- Patch updates auto-merge
- Minor/major updates require review

### 4. Docker Images in Workflows
Custom manager tracks:
- `moby/buildkit:v0.19.0` in CI workflows
- Automatically updates to latest stable version

### 5. Tool Versions (mise.toml)
Custom manager tracks:
- Go version
- golangci-lint version

## Workflow Integration

```
Renovate detects update
        ↓
Creates PR (Chart.yaml updated)
        ↓
patch-on-pr.yaml workflow runs
        ↓
Commits patched charts to PR
        ↓
Ready to merge!
```

## Configuration Highlights

### Grouping
- All Helm charts grouped together in one PR
- Go dependencies grouped together
- GitHub Actions grouped together
- Reduces PR noise

### Scheduling
- Runs before 4am UTC on Mondays
- Security updates run immediately
- Max 3 concurrent PRs to avoid overwhelming CI

### Auto-merge
✅ Auto-merged:
- Go minor/patch updates
- GitHub Actions patch updates
- Security vulnerability fixes

⚠️ Requires review:
- Helm chart updates (need to verify patching works)
- Major version updates
- Breaking changes

## Labels

PRs are automatically labeled:
- `dependencies` - All dependency updates
- `helm` - Helm chart updates
- `go` - Go dependency updates
- `github-actions` - GitHub Actions updates
- `docker` - Docker image updates
- `security` - Security vulnerability fixes

## Enabling Renovate

### For GitHub.com repositories:

1. **Install Renovate App:**
   - Visit https://github.com/apps/renovate
   - Click "Install"
   - Select this repository

2. **Or enable GitHub-native Dependency Graph:**
   - Repository Settings → Security → Dependency graph
   - Enable Dependabot alerts

### For Self-Hosted:

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
      "automerge": false  // ← Change to false
    }
  ]
}
```

### Add More Custom Managers

Track additional version patterns:
```json
{
  "customManagers": [
    {
      "customType": "regex",
      "fileMatch": ["^Dockerfile$"],
      "matchStrings": [
        "FROM (?<depName>\\S+):(?<currentValue>\\S+)"
      ],
      "datasourceTemplate": "docker"
    }
  ]
}
```

## How It Works With Workflows

### Scenario 1: Helm Chart Update

```
1. Monday 4am UTC - Renovate checks for updates
2. Finds prometheus 25.8.0 → 25.9.0
3. Creates PR updating Chart.yaml
4. patch-on-pr.yaml triggers:
   - Pulls prometheus 25.9.0
   - Scans for vulnerabilities
   - Patches images
   - Creates wrapper chart
   - Commits to PR branch
5. PR ready for review with:
   - Updated Chart.yaml ✓
   - Patched wrapper chart ✓
   - Trivy reports ✓
```

### Scenario 2: Security Vulnerability

```
1. New CVE published
2. Renovate immediately creates PR
3. automerge: true → Auto-merges after CI passes
4. Merged to main
5. publish.yaml pushes updated images to Quay.io
```

### Scenario 3: BuildKit Update

```
1. moby/buildkit v0.19.0 → v0.20.0
2. Renovate updates .github/workflows/ci.yaml
3. PR created for review
4. Merge updates all workflows
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

### Custom manager not working

Debug:
```bash
renovate --dry-run --log-level=debug
```

Look for "customManager" in logs to see if patterns match.

## Related Documentation

- [WORKFLOWS.md](../WORKFLOWS.md) - How workflows integrate with Renovate
- [WRAPPER_CHARTS.md](../WRAPPER_CHARTS.md) - What gets updated in PRs
- [Renovate Docs](https://docs.renovatebot.com/) - Full documentation
