cask "dialex-ai" do
  version "1.0.0"
  sha256 :no_check # Updated upon official release

  url "https://github.com/Kolta-Labs/DialexAI/releases/download/v#{version}/DialexAI-#{version}-without-engine.dmg"
  name "Dialex AI"
  desc "Sovereign Multi-Agent AI Deliberation Platform (Client App)"
  homepage "https://dialex.ai"

  app "DialexAI.app"

  zap trash: [
    "~/Library/Application Support/Dialex",
    "~/Library/Preferences/com.koltalabs.dialex.plist",
    "~/Library/Saved Application State/com.koltalabs.dialex.savedState",
  ]
end
