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

    # Ruby interpolation uses #{...} — we build the formula in two parts to avoid
    # Python f-string trying to evaluate #{arch} and #{bin} as Python expressions.
    ruby_arch_interp = "#{arch}"    # noqa: E501  — this is a Ruby string, not Python
    ruby_bin_interp  = "#{bin}"     # noqa: E501

    formula = (
        "# typed: false\n"
        "# frozen_string_literal: true\n"
        "\n"
        "class Evoclaw < Formula\n"
        '  desc "Self-evolving AI agent framework"\n'
        '  homepage "https://github.com/clawinfra/evoclaw"\n'
        f'  version "{version}"\n'
        '  license "MIT"\n'
        "\n"
        "  on_macos do\n"
        "    if Hardware::CPU.arm?\n"
        f'      url "{base_url}/evoclaw-darwin-arm64.tar.gz"\n'
        f'      sha256 "{arm64_sha}"\n'
        "    else\n"
        f'      url "{base_url}/evoclaw-darwin-amd64.tar.gz"\n'
        f'      sha256 "{amd64_sha}"\n'
        "    end\n"
        "  end\n"
        "\n"
        "  def install\n"
        '    arch = Hardware::CPU.arm? ? "arm64" : "amd64"\n'
        f'    bin.install "evoclaw-darwin-{ruby_arch_interp}" => "evoclaw"\n'
        "  end\n"
        "\n"
        "  test do\n"
        f'    assert_match version.to_s, shell_output("{ruby_bin_interp}/evoclaw version")\n'
        "  end\n"
        "end\n"
    )

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
