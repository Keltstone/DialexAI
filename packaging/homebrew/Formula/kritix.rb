class Kritix < Formula
  desc "Kritix AI — Autonomous QA Testing Studio & Enterprise Verification CLI"
  homepage "https://dialex.ai"
  url "https://github.com/Kolta-Labs/DialexAI/archive/refs/tags/v1.0.0.tar.gz"
  version "1.0.0"
  license "Apache-2.0"
  head "https://github.com/Kolta-Labs/DialexAI.git", branch: "main"

  option "with-engine", "Install with the Socratix deliberation engine"

  depends_on "go" => :build
  depends_on "socratix" => :recommended if build.with? "engine"

  def install
    cd "kritix-ai" do
      system "go", "build", "-ldflags=-s -w -X main.Version=#{version}", "-o", bin/"kritix", "./cmd/kritix"
    end
  end

  def post_install
    engine_bin = which("socratix") || [
      "/usr/local/bin/socratix",
      "#{Dir.home}/.local/bin/socratix",
      "#{HOMEBREW_PREFIX}/bin/socratix"
    ].find { |p| File.executable?(p) }

    if engine_bin
      puts "✓ Connected to existing Socratix engine at: #{engine_bin}"
    elsif !build.with?("engine")
      puts ""
      opoo "No Socratix engine detected. Kritix will use internal heuristics."
      puts "To install the local deliberation engine for deep consensus analysis:"
      puts "  brew install socratix"
      puts "or reinstall Kritix with:"
      puts "  brew install kritix --with-engine"
    end
  end

  test do
    assert_match "Kritix AI Engine", shell_output("#{bin}/kritix version")
  end
end
