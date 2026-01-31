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

## Instructions

### Step 1: Check Current State

```bash
# Current version
cat cmd/penf/VERSION

# Ensure working directory is clean for VERSION file
git status cmd/penf/VERSION

# Check latest release on GitHub
gh release list --limit 3
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

```bash
git add cmd/penf/VERSION
git commit -m "chore: release <NEW_VERSION>

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
git push
```

### Step 6: Monitor Release

The push triggers GitHub Actions. Show the user how to monitor:

```
Release triggered for <NEW_VERSION>

Monitor progress:
  gh run list --workflow=auto-release.yml --limit 1
  gh run watch

Or view in browser:
  https://github.com/otherjamesbrown/penfold/actions

Once complete, update locally:
  penf update
```

Optionally watch the workflow:

```bash
# Get the run ID
gh run list --workflow=auto-release.yml --limit 1 --json databaseId -q '.[0].databaseId'

# Watch it
gh run watch <RUN_ID>
```

### Step 7: Verify Release

After workflow completes:

```bash
# Check release exists
gh release view <NEW_VERSION>

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

GitHub Actions triggered. Release will be available in ~2-3 minutes.

Monitor: gh run watch
Update:  penf update
Release: https://github.com/otherjamesbrown/penfold/releases/tag/<NEW_VERSION>
```

## Notes

- Only changes to `cmd/penf/VERSION` on main trigger the release
- Pre-release versions (containing -alpha, -beta, -rc) are marked as prerelease
- The workflow builds for: darwin-arm64, darwin-amd64, linux-amd64, linux-arm64
- Users update with `penf update` which downloads from GitHub Releases
