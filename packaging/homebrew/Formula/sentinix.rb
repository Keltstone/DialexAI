class Sentinix < Formula
  desc "Sentinix AI — Continuous Security Review, PR Gate & Vulnerability Posture Engine"
  homepage "https://dialex.ai"
  url "https://github.com/Keltstone/DialexAI/archive/refs/tags/sentinix-v1.0.0.tar.gz"
  version "1.0.0"
  license "Apache-2.0"
  head "https://github.com/Keltstone/DialexAI.git", branch: "main"

  depends_on "go" => :build

  def install
    cd "sentinix-ai" do
      system "go", "build", "-ldflags=-s -w -X main.version=#{version}", "-o", bin/"sentinix", "./cmd/sentinix"
      system "go", "build", "-ldflags=-s -w -X main.version=#{version}", "-o", bin/"sentinixd", "./cmd/sentinixd"
    end
  end

  def caveats
    <<~EOS
      Sentinix AI CLI and Daemon installed!

      To scan a repository locally:
        sentinix scan /path/to/project

      To run the PR review server & dashboard:
        sentinixd -config sentinix.json -db sqlite:sentinix.db -addr :8080
    EOS
  end

  test do
    assert_match "Sentinix AI version #{version}", shell_output("#{bin}/sentinix version")
  end
end
