cask "kritix-ai-bundled" do
  version "1.0.0"
  sha256 :no_check

  url "https://github.com/Kolta-Labs/DialexAI/releases/download/v#{version}/KritixAI-#{version}-with-engine.dmg"
  name "Kritix AI (Bundled Engine)"
  desc "Autonomous QA Testing Studio & Cockpit (Bundled with Socratix Engine)"
  homepage "https://dialex.ai"

  conflicts_with cask: "kritix-ai"

  app "KritixAI.app"

  postflight do
    existing_engine = [
      "/usr/local/bin/socratix",
      "/opt/homebrew/bin/socratix",
      "#{Dir.home}/.local/bin/socratix"
    ].find { |p| File.executable?(p) }

    if existing_engine
      opoo "Notice: An external Socratix deliberation engine was detected at #{existing_engine}."
      puts "Kritix AI bundles an embedded Socratix engine, but it will automatically"
      puts "detect and attach to your existing engine on port 7890 if active."
    end
  end

  zap trash: [
    "~/.kritix",
    "~/Library/Application Support/Kritix",
    "~/Library/Preferences/com.koltalabs.kritix.plist",
    "~/Library/Saved Application State/com.koltalabs.kritix.savedState",
  ]
end
