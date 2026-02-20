#!/usr/bin/env python3
"""Update Formula/evoclaw.rb in a checked-out homebrew-evoclaw repo.

Usage:
    python3 update-homebrew-formula.py <version> <tag> <amd64_sha> <arm64_sha>

Example:
    python3 update-homebrew-formula.py 0.3.1 v0.3.1 abc123... def456...
"""

import sys
import os

def main():
    if len(sys.argv) != 5:
        print(f"Usage: {sys.argv[0]} <version> <tag> <amd64_sha> <arm64_sha>")
        sys.exit(1)

    version  = sys.argv[1]   # e.g. 0.3.1
    tag      = sys.argv[2]   # e.g. v0.3.1
    amd64_sha = sys.argv[3]
    arm64_sha = sys.argv[4]

    base_url = f"https://github.com/clawinfra/evoclaw/releases/download/{tag}"

    formula = f"""\
# typed: false
# frozen_string_literal: true

class Evoclaw < Formula
  desc "Self-evolving AI agent framework"
  homepage "https://github.com/clawinfra/evoclaw"
  version "{version}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "{base_url}/evoclaw-darwin-arm64.tar.gz"
      sha256 "{arm64_sha}"
    else
      url "{base_url}/evoclaw-darwin-amd64.tar.gz"
      sha256 "{amd64_sha}"
    end
  end

  def install
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    bin.install "evoclaw-darwin-\#{arch}" => "evoclaw"
  end

  test do
    assert_match version.to_s, shell_output("\#{bin}/evoclaw version")
  end
end
"""

    formula_path = os.path.join("Formula", "evoclaw.rb")
    os.makedirs("Formula", exist_ok=True)
    with open(formula_path, "w") as f:
        f.write(formula)

    print(f"✅ Formula written to {formula_path}")
    print(f"   Version: {version}")
    print(f"   amd64:   {amd64_sha[:16]}...")
    print(f"   arm64:   {arm64_sha[:16]}...")


if __name__ == "__main__":
    main()
