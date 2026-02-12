# Homebrew Auto-Release Setup

This workflow automatically updates the Homebrew tap when a new release is published.

## Required Secret

The workflow needs a GitHub Personal Access Token with write access to the `homebrew-evoclaw` repo.

### Creating the Token

1. Go to https://github.com/settings/tokens/new
2. Select **Fine-grained tokens** (recommended)
3. Configure:
   - **Name:** `Homebrew Tap Auto-Update`
   - **Expiration:** 1 year (or no expiration)
   - **Repository access:** Only select repositories → `clawinfra/homebrew-evoclaw`
   - **Permissions:**
     - Contents: Read and write
     - Metadata: Read-only (automatic)
4. Click **Generate token**
5. Copy the token (starts with `github_pat_...`)

### Adding the Secret

1. Go to https://github.com/clawinfra/evoclaw/settings/secrets/actions
2. Click **New repository secret**
3. Name: `HOMEBREW_TAP_TOKEN`
4. Value: Paste the token
5. Click **Add secret**

## How It Works

When a release is published (e.g., `v1.0.0`):

1. **Download binaries:** Gets `evoclaw-darwin-{amd64,arm64}.tar.gz` from release
2. **Calculate SHA256:** Computes checksums for both macOS binaries
3. **Update formula:** Replaces version and SHA256 in `Formula/evoclaw.rb`
4. **Test:** Validates Ruby syntax
5. **Commit & push:** Auto-commits to `homebrew-evoclaw` repo

Users see the update immediately on `brew update`.

## Testing

To test without releasing:

```bash
# Create a draft release
gh release create v1.0.0-test --draft --title "Test Release" --notes "Testing Homebrew automation"

# Upload test binaries
gh release upload v1.0.0-test evoclaw-darwin-amd64.tar.gz evoclaw-darwin-arm64.tar.gz

# Publish (triggers workflow)
gh release edit v1.0.0-test --draft=false

# Check workflow
gh run list --workflow=homebrew-release.yml

# Clean up
gh release delete v1.0.0-test --yes
```

## Troubleshooting

**Workflow fails with "Resource not accessible by integration":**
- Check that `HOMEBREW_TAP_TOKEN` secret exists
- Verify token has write access to `homebrew-evoclaw`
- Token may have expired (regenerate if needed)

**Formula update fails:**
- Check that macOS binaries exist in the release
- Verify binary names: `evoclaw-darwin-amd64.tar.gz` and `evoclaw-darwin-arm64.tar.gz`
- Ensure release is not a draft

**Users report wrong checksums:**
- GitHub releases are immutable, but downloads can be cached
- Wait 5-10 minutes for CDN to update
- Or delete and re-upload the release assets

## Manual Override

If automation fails, follow manual steps in `docs/HOMEBREW-RELEASE.md`.
