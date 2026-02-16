# Running EvoClaw on macOS Without Code Signing

This guide covers how to run EvoClaw on macOS without an Apple Developer certificate (unsigned binaries).

## What to Expect

### Gatekeeper Warning

When you first run an unsigned EvoClaw binary, macOS Gatekeeper will block it:

```
"evoclaw" cannot be opened because it is from an unidentified developer.
```

**This is normal and expected.** The binary is safe — it's just not signed by Apple.

---

## Workaround Options

Choose the method that works best for you:

### Option 1: Right-Click Method (Recommended)

**Easiest for .dmg or .app bundles:**

1. Right-click (or Control+click) on `EvoClaw.app`
2. Click **"Open"** from the menu
3. Click **"Open"** again in the dialog that appears
4. ✅ EvoClaw runs — macOS remembers this choice permanently

**One-time setup, works forever.**

### Option 2: System Settings

**For binaries downloaded from the web:**

1. Try to open EvoClaw (you'll get blocked)
2. Go to **System Settings → Privacy & Security**
3. Scroll down to the Security section
4. Click **"Open Anyway"** next to the blocked app message
5. Confirm by clicking **"Open"**
6. ✅ Done

**Appears only for the last blocked app, so do this immediately after the first attempt.**

### Option 3: Terminal Command

**For CLI users:**

```bash
# Remove quarantine flag from downloaded binary
xattr -d com.apple.quarantine /path/to/evoclaw

# Or if installed via .dmg:
xattr -d com.apple.quarantine /Applications/EvoClaw.app

# Now run normally
./evoclaw --version
```

**What this does:** Removes the "downloaded from internet" flag that triggers Gatekeeper.

### Option 4: Disable Gatekeeper Globally (Not Recommended)

```bash
# Disable Gatekeeper entirely (security risk)
sudo spctl --master-disable

# Re-enable later with:
sudo spctl --master-enable
```

**⚠️ Warning:** This disables macOS security checks for ALL apps. Only use if you know what you're doing.

### Option 5: Homebrew (Best for Developers)

```bash
brew tap clawinfra/evoclaw
brew install evoclaw
```

**Why this works:** Homebrew-installed binaries don't trigger Gatekeeper warnings at all. No workarounds needed.

---

## Recommended Installation by User Type

### Developers / Power Users
→ **Use Homebrew**
```bash
brew tap clawinfra/evoclaw
brew install evoclaw
evoclaw init
```
- No warnings
- Easy updates: `brew upgrade evoclaw`
- Familiar workflow

### Non-Technical Users
→ **Use .dmg installer + Right-click method**
1. Download `EvoClaw-{version}-arm64.dmg`
2. Open DMG, drag to Applications
3. Right-click EvoClaw.app → Open → Open
4. Done

### Terminal-Only Users
→ **Download binary + xattr**
```bash
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-darwin-arm64.tar.gz
tar -xzf evoclaw-darwin-arm64.tar.gz
sudo mv evoclaw-darwin-arm64 /usr/local/bin/evoclaw
xattr -d com.apple.quarantine /usr/local/bin/evoclaw
evoclaw init
```

### Enterprise / IT Departments
→ **Wait for signed releases** (requires Apple Developer certificate)

---

## Comparison: Unsigned vs Signed

| Feature | Unsigned (Current) | Signed (Future) |
|---------|-------------------|-----------------|
| **First run experience** | Gatekeeper warning (10s workaround) | Opens immediately |
| **Updates** | Manual download + repeat workaround | Seamless auto-update |
| **Trust indicator** | "Unidentified developer" | "Verified by ClawInfra" |
| **Distribution** | GitHub, Homebrew | GitHub, Homebrew, Mac App Store |
| **Enterprise deployment** | Requires IT policy exception | Works out-of-box |
| **Cost** | Free | $99/year (Apple Developer) |

---

## When We'll Add Code Signing

We'll add Apple Developer code signing when:

1. **User base grows** — When we see significant macOS adoption
2. **Non-technical users** — When mainstream (non-developer) users are primary audience
3. **Enterprise requests** — When companies want to deploy EvoClaw at scale
4. **Mac App Store** — If we pursue App Store distribution

**For now:** Our audience is primarily developers and DevOps engineers who are comfortable with unsigned binaries and use Homebrew.

---

## Verifying Binary Integrity

Even without code signing, you can verify the download:

### 1. Check SHA256 Checksum

```bash
# Download checksum file
curl -LO https://github.com/clawinfra/evoclaw/releases/download/v0.2.0/SHA256SUMS

# Verify your download
shasum -a 256 -c SHA256SUMS --ignore-missing
```

Should output:
```
evoclaw-darwin-arm64.tar.gz: OK
```

### 2. Inspect the Binary

```bash
# Check if it's a valid Mach-O executable
file evoclaw
# Output: evoclaw: Mach-O 64-bit executable arm64

# Check for malicious code (basic)
strings evoclaw | grep -i "malware\|keylog\|backdoor"
# (Should return nothing)

# Verify it came from GitHub
xattr -l evoclaw | grep com.apple.quarantine
# Shows download source (should be github.com)
```

### 3. Run in Sandbox (Advanced)

```bash
# Test in a macOS sandbox container
sandbox-exec -f /usr/share/sandbox/bsd.sb ./evoclaw --version
```

---

## Troubleshooting

### "Operation not permitted" when running xattr

**Cause:** System Integrity Protection (SIP) prevents modifying certain files.

**Solution:** Run xattr on your downloaded copy, not system files:
```bash
cp /path/to/downloaded/evoclaw ~/evoclaw
xattr -d com.apple.quarantine ~/evoclaw
./~/evoclaw --version
```

### Gatekeeper warning persists after right-click

**Cause:** The quarantine attribute is still set.

**Solution:** Clear it manually:
```bash
xattr -d com.apple.quarantine /Applications/EvoClaw.app
```

### "Binary is damaged and can't be opened"

**Cause:** Incomplete download or corrupted file.

**Solution:** 
1. Re-download the file
2. Verify SHA256 checksum
3. Try a different browser (Safari sometimes mangles downloads)

### Homebrew installation fails

**Cause:** Tap not added or formula not found.

**Solution:**
```bash
# Remove old tap if exists
brew untap clawinfra/evoclaw

# Re-add tap
brew tap clawinfra/evoclaw

# Verify formula
brew info evoclaw

# Install
brew install evoclaw
```

---

## FAQ

### Is it safe to run unsigned binaries?

**Short answer:** Yes, if you trust the source.

**Long answer:** Code signing proves identity, not safety. An unsigned binary from a trusted source (like EvoClaw on GitHub) can be just as safe as a signed one. We provide:
- Open-source code (auditable)
- SHA256 checksums (verify integrity)
- GitHub releases (traceable provenance)

### Why not just get an Apple Developer account?

**We will.** But it costs $99/year + time to set up, and our current audience (developers) doesn't need it. We're prioritizing:
1. Soft launch to technical users (now)
2. Gather feedback and iterate
3. Get code signing when we go mainstream (2-3 weeks)

### Will signed releases work differently?

No. Functionally identical. The only difference is the first-run experience:
- **Unsigned:** "Unidentified developer" warning → right-click to open
- **Signed:** Opens immediately, shows "Verified by ClawInfra"

### Can I use EvoClaw in production without signing?

**Yes.** Many production deployments run unsigned binaries (especially in DevOps/SRE contexts). If you're comfortable with the workaround methods, go for it.

For enterprise environments with strict policies, wait for signed releases.

### What about Windows and Linux?

**Windows:** Similar situation. Unsigned .exe files show SmartScreen warnings. Users can click "More info" → "Run anyway." We'll add Windows code signing ($200-400/year) alongside Apple.

**Linux:** No code signing required. .deb and .rpm packages work out-of-box on all distros.

---

## Summary

**Unsigned EvoClaw is perfectly usable.** Choose your method:

| Method | Best For | Time |
|--------|----------|------|
| **Homebrew** | Developers | 0s (no warnings) |
| **Right-click** | Regular users | 10s (one-time) |
| **xattr** | CLI users | 5s (one-time) |
| **System Settings** | Anyone | 15s (one-time) |

All methods are safe and effective. We'll add code signing in 2-3 weeks for mainstream users.

**Questions?** Open an issue: https://github.com/clawinfra/evoclaw/issues
