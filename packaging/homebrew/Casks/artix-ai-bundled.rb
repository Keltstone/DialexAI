cask "artix-ai-bundled" do
  version "1.0.0"
  sha256 :no_check

  url "https://github.com/Keltstone/DialexAI/releases/download/v#{version}/ArtixAI-#{version}-with-engine.dmg"
  name "Artix AI (Bundled Engine)"
  desc "Autonomous Software Engineering & Spec-to-Patch Cockpit (Bundled with Socratix Engine)"
  homepage "https://dialex.ai"

  conflicts_with cask: "artix-ai"

  app "ArtixAI.app"

  postflight do
    existing_engine = [
      "/usr/local/bin/socratix",
      "/opt/homebrew/bin/socratix",
      "#{Dir.home}/.local/bin/socratix"
    ].find { |p| File.executable?(p) }

    if existing_engine
      opoo "Notice: An external Socratix deliberation engine was detected at #{existing_engine}."
      puts "Artix AI bundles an embedded Socratix engine, but it will automatically"
      puts "detect and attach to your existing engine on port 7890 if active."
    end
  end

  zap trash: [
    "~/.artix",
    "~/Library/Application Support/Artix",
    "~/Library/Preferences/com.koltalabs.artix.plist",
    "~/Library/Saved Application State/com.koltalabs.artix.savedState",
  ]
end
