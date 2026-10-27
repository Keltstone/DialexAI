cask "kritix-ai" do
  version "1.0.0"
  sha256 :no_check

  url "https://github.com/Keltstone/DialexAI/releases/download/v#{version}/KritixAI-#{version}-without-engine.dmg"
  name "Kritix AI"
  desc "Autonomous QA Testing Studio & Enterprise Verification Cockpit (Client App)"
  homepage "https://dialex.ai"

  conflicts_with cask: "kritix-ai-bundled"

  app "KritixAI.app"

  zap trash: [
    "~/.kritix",
    "~/Library/Application Support/Kritix",
    "~/Library/Preferences/com.keltstone.kritix.plist",
    "~/Library/Saved Application State/com.keltstone.kritix.savedState",
  ]
end
