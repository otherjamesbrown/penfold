# CLI Publish

Build and publish a new version of the penf CLI to GitHub Releases.

## Arguments: $ARGUMENTS

Optional: Version bump type or specific version
- `patch` (default) - v0.3.8 -> v0.3.9
- `minor` - v0.3.8 -> v0.4.0
- `major` - v0.3.8 -> v1.0.0
- `v1.2.3` - Set specific version

## How It Works

The penf CLI is released via GitHub Actions:

1. **VERSION file** (`cmd/penf/VERSION`) contains the current version
2. **auto-release.yml** watches for changes to VERSION on main branch
3. When VERSION changes:
   - Creates git tag with that version
   - Triggers release.yml workflow
4. **release.yml** builds binaries for darwin-arm64, darwin-amd64, linux-amd64, linux-arm64
5. Creates GitHub Release with binaries and checksums
6. Users update with `penf update`

**Note:** Only git push is required (SSH auth works). No `gh` CLI needed.

## Instructions

### Step 1: Check Current State

```bash
# Current version
cat cmd/penf/VERSION

# Ensure working directory is clean for VERSION file
git status cmd/penf/VERSION

# Check latest release on GitHub (no auth required)
curl -s https://api.github.com/repos/otherjamesbrown/penfold/releases/latest | grep tag_name
```

### Step 2: Determine New Version

Parse the argument to determine the new version:

- If no argument or `patch`: increment patch version (v0.3.8 -> v0.3.9)
- If `minor`: increment minor, reset patch (v0.3.8 -> v0.4.0)
- If `major`: increment major, reset minor and patch (v0.3.8 -> v1.0.0)
- If starts with `v`: use that exact version

Calculate the new version and confirm with user:

```
Current version: v0.3.8
New version:     v0.3.9

Continue? (y/n)
```

### Step 3: Build and Verify

Build locally to ensure it compiles:

```bash
# Build with new version
go build -ldflags "-X main.version=<NEW_VERSION>" -o penf ./cmd/penf/

# Verify it runs
./penf version
```

If build fails, stop and report the error.

### Step 4: Update VERSION File

```bash
echo "<NEW_VERSION>" > cmd/penf/VERSION
```

### Step 5: Commit and Push

Git push triggers the release (SSH auth is sufficient):

```bash
git add cmd/penf/VERSION
git commit -m "chore: release <NEW_VERSION>

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
git push
```

### Step 6: Monitor Release

The push triggers GitHub Actions automatically.

**View in browser:**
https://github.com/otherjamesbrown/penfold/actions/workflows/auto-release.yml

**Or check via API (no auth):**
```bash
# Wait ~30 seconds, then check if tag was created
curl -s https://api.github.com/repos/otherjamesbrown/penfold/tags | grep -A1 '"name"' | head -4

# Check release status (~2-3 min after push)
curl -s https://api.github.com/repos/otherjamesbrown/penfold/releases/latest | grep tag_name
```

### Step 7: Verify Release

After workflow completes (~2-3 minutes):

```bash
# Update local CLI
penf update

# Verify
penf version
```

## Output Summary

```
CLI Release: <NEW_VERSION>

Previous: <OLD_VERSION>
New:      <NEW_VERSION>

Pushed to main. GitHub Actions will:
1. Create tag <NEW_VERSION>
2. Build binaries (darwin/linux, arm64/amd64)
3. Publish release

Monitor: https://github.com/otherjamesbrown/penfold/actions
Release: https://github.com/otherjamesbrown/penfold/releases/tag/<NEW_VERSION>

Update locally (after ~2-3 min):
  penf update
```

## Notes

- Only git push required - SSH auth is sufficient, no `gh` CLI needed
- Only changes to `cmd/penf/VERSION` on main trigger the release
- Pre-release versions (containing -alpha, -beta, -rc) are marked as prerelease
- The workflow builds for: darwin-arm64, darwin-amd64, linux-amd64, linux-arm64
- Users update with `penf update` which downloads from GitHub Releases
