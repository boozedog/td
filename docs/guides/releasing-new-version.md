# Releasing a New Version

Guide for creating new td releases.

## Prerequisites

- Clean working tree (`git status` shows no changes)
- All tests passing (`go test ./...`)
- On the main branch
- GitHub CLI authenticated (`gh auth status`)

## Release Process

### 1. Determine Version

Follow semantic versioning:
- **Major** (v1.0.0): Breaking changes
- **Minor** (v0.5.0): New features, backward compatible
- **Patch** (v0.4.26): Bug fixes only

Check current version:
```bash
git tag -l | sort -V | tail -1
```

### 2. Verify Tests Pass

```bash
go test ./...
```

### 3. Create and Push Tag

```bash
# Create annotated tag
git tag -a vX.Y.Z -m "Release vX.Y.Z: brief description"

# Push commit and tag
git push origin main
git push origin vX.Y.Z
```

### 4. Create GitHub Release

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes "$(cat <<'EOF'
## What's New

### Features
- Feature description

### Bug Fixes
- Fix description

EOF
)"
```

Or create interactively:
```bash
gh release create vX.Y.Z --title "vX.Y.Z" --generate-notes
```

### 5. Install Locally with Version

```bash
go install -ldflags "-X main.Version=vX.Y.Z" ./...
```

### 6. Verify

```bash
# Check release exists
gh release view vX.Y.Z

# Verify local version
td version
```

## Version in Binaries

Version is embedded at build time via ldflags:

```bash
# Build with specific version
go build -ldflags "-X main.Version=v0.4.26" -o td .

# Install with version
go install -ldflags "-X main.Version=v0.4.26" ./...
```

Without ldflags, version defaults to `devel` or shows git info.

## Update Mechanism

Users see update notifications because:
1. On `td version`, it checks `https://api.github.com/repos/marcus/td/releases/latest`
2. Compares `tag_name` against current version
3. Shows update command if newer version exists
4. Results cached for 6 hours in `~/.cache/td/version-cache.json`

Dev versions (`devel`, `devel+hash`) skip the check.

## Quick Release (Copy-Paste)

Replace `X.Y.Z` with actual version:

```bash
# Verify clean state
git status
go test ./...

# Tag and push
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin main && git push origin vX.Y.Z

# Create release
gh release create vX.Y.Z --title "vX.Y.Z" --generate-notes

# Install locally
go install -ldflags "-X main.Version=vX.Y.Z" ./...

# Verify
td version
```

## Checklist

- [ ] Tests pass (`go test ./...`)
- [ ] Working tree clean
- [ ] Version number follows semver
- [ ] Tag created with `-a` (annotated)
- [ ] Tag pushed to origin
- [ ] GitHub release created
- [ ] Local install updated with version
- [ ] `td version` shows correct version
