class Socratix < Formula
  desc "Socratix — Multi-agent consensus & deliberation engine by Keltstone Systems"
  homepage "https://dialex.ai"
  url "https://github.com/Keltstone/DialexAI/archive/refs/tags/v1.0.0.tar.gz"
  version "1.0.0"
  license "Apache-2.0"
  head "https://github.com/Keltstone/DialexAI.git", branch: "main"

  depends_on "go" => :build

  def install
    cd "socratix-engine" do
      system "go", "build", "-ldflags=-s -w -X main.version=#{version}", "-o", bin/"socratix", "./cmd/socratix"
      system "go", "build", "-ldflags=-s -w", "-o", bin/"socratixbench", "./cmd/socratixbench"
    end
  end

  def post_install
    # Check whether an engine is already installed outside Homebrew
    external_candidates = [
      "/usr/local/bin/socratix",
      "#{Dir.home}/.local/bin/socratix"
    ].select { |p| File.executable?(p) && File.realpath(p) != (bin/"socratix").to_s rescue false }

    if external_candidates.any?
      opoo "Existing Socratix engine binary detected outside Homebrew: #{external_candidates.join(", ")}"
      puts ""
      puts "Homebrew has installed socratix to: #{bin}/socratix"
      puts "Your PATH currently resolves to the external binary. If you wish to switch to Homebrew's binary,"
      puts "either update your PATH or run:"
      puts "  brew link --overwrite socratix"
    end
  end

  def caveats
    <<~EOS
      Socratix engine installed!
      To start the local deliberation daemon on port 7890:
        socratix serve

      To install as a background login service:
        socratix service install
    EOS
  end

  test do
    assert_match "socratix", shell_output("#{bin}/socratix --help 2>&1", 1)
  end
end
