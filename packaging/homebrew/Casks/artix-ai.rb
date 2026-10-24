cask "artix-ai" do
  version "1.0.0"
  sha256 :no_check

  url "https://github.com/Keltstone/DialexAI/releases/download/v#{version}/ArtixAI-#{version}-without-engine.dmg"
  name "Artix AI"
  desc "Autonomous Software Engineering & Spec-to-Patch Deliberation Cockpit (Client App)"
  homepage "https://dialex.ai"

  conflicts_with cask: "artix-ai-bundled"

  app "ArtixAI.app"

  zap trash: [
    "~/.artix",
    "~/Library/Application Support/Artix",
    "~/Library/Preferences/com.koltalabs.artix.plist",
    "~/Library/Saved Application State/com.koltalabs.artix.savedState",
  ]
end
