cask "dialex-ai-bundled" do
  version "1.0.0"
  sha256 :no_check # Updated upon official release

  url "https://github.com/Kolta-Labs/DialexAI/releases/download/v#{version}/DialexAI-#{version}-with-engine.dmg"
  name "Dialex AI (Bundled Engine)"
  desc "Sovereign Multi-Agent AI Deliberation Platform (Bundled with Socratix Engine)"
  homepage "https://dialex.ai"

  conflicts_with cask: "dialex-ai"

  app "DialexAI.app"

  postflight do
    # Check whether an existing engine is installed on the host
    existing_engine = [
      "/usr/local/bin/socratix",
      "/opt/homebrew/bin/socratix",
      "#{Dir.home}/.local/bin/socratix"
    ].find { |p| File.executable?(p) }

    if existing_engine
      opoo "Notice: An external Socratix engine was detected at #{existing_engine}."
      puts "Dialex AI includes an embedded Socratix engine, but it will automatically"
      puts "detect and attach to your existing engine on port 7890 if active."
    end
  end

  zap trash: [
    "~/Library/Application Support/Dialex",
    "~/Library/Preferences/com.koltalabs.dialex.plist",
    "~/Library/Saved Application State/com.koltalabs.dialex.savedState",
  ]
end
