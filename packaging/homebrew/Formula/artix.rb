class Artix < Formula
  desc "Artix AI — Dialectic Software Engineering & Autonomous Coding CLI"
  homepage "https://dialex.ai"
  url "https://github.com/Kolta-Labs/DialexAI/archive/refs/tags/v1.0.0.tar.gz"
  version "1.0.0"
  license "Apache-2.0"
  head "https://github.com/Kolta-Labs/DialexAI.git", branch: "main"

  option "with-engine", "Install with the Socratix deliberation engine"

  depends_on "go" => :build
  depends_on "socratix" => :recommended if build.with? "engine"

  def install
    cd "artix-ai" do
      system "go", "build", "-ldflags=-s -w -X main.version=#{version}", "-o", bin/"artix", "./cli"
      system "go", "build", "-ldflags=-s -w", "-o", bin/"artixd", "./cmd/artixd"
    end
  end

  def post_install
    # Check whether socratix engine is installed when needed
    engine_bin = which("socratix") || [
      "/usr/local/bin/socratix",
      "#{Dir.home}/.local/bin/socratix",
      "#{HOMEBREW_PREFIX}/bin/socratix"
    ].find { |p| File.executable?(p) }

    if engine_bin
      puts "✓ Connected to existing Socratix engine at: #{engine_bin}"
    elsif !build.with?("engine")
      puts ""
      opoo "No Socratix engine detected. Artix CLI will operate in standalone mode."
      puts "To install the local deliberation engine, run:"
      puts "  brew install socratix"
      puts "or reinstall Artix with:"
      puts "  brew install artix --with-engine"
    end
  end

  test do
    assert_match "Artix AI version #{version}", shell_output("#{bin}/artix version")
  end
end
