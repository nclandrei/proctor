#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 5 ]; then
  echo "usage: $0 <version> <darwin-arm64-sha256> <darwin-amd64-sha256> <linux-amd64-sha256> <output-path>" >&2
  exit 1
fi

version="$1"
darwin_arm64_sha="$2"
darwin_amd64_sha="$3"
linux_amd64_sha="$4"
output_path="$5"

mkdir -p "$(dirname "$output_path")"

cat >"$output_path" <<EOF
class Proctor < Formula
  desc "Manual verification contract CLI for coding agents"
  homepage "https://github.com/nclandrei/proctor"
  version "${version}"

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/nclandrei/proctor/releases/download/v${version}/proctor-aarch64-apple-darwin.tar.gz"
      sha256 "${darwin_arm64_sha}"
    end
    if Hardware::CPU.intel?
      url "https://github.com/nclandrei/proctor/releases/download/v${version}/proctor-x86_64-apple-darwin.tar.gz"
      sha256 "${darwin_amd64_sha}"
    end
  end
  if OS.linux?
    if Hardware::CPU.intel?
      url "https://github.com/nclandrei/proctor/releases/download/v${version}/proctor-x86_64-unknown-linux-gnu.tar.gz"
      sha256 "${linux_amd64_sha}"
    end
  end

  def install
    bin.install "proctor"
  end

  test do
    output = shell_output("#{bin}/proctor --help")
    assert_match "make a coding agent prove it manually tested its own work", output
    assert_match "Codex, Claude Code", output
  end
end
EOF
