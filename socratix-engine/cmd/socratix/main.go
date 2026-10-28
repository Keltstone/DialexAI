// Command socratix is the engine binary — `socratix serve` runs the HTTP daemon;
// `socratix users add` seeds the multi-user account store.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"socratix/pkg/api"
	"socratix/pkg/service"
	"socratix/pkg/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "users":
		runUsers(os.Args[2:])
	case "service":
		runService(os.Args[2:])
	case "ask":
		runAsk(os.Args[2:])
	case "consensus":
		runConsensus(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`socratix — the Socratix consensus & deliberation engine by Keltstone Systems

Usage:
  socratix serve [flags]        Run the HTTP/SSE daemon
  socratix users add <name>     Add an account to the multi-user store (--dir to target
                              a non-default store, e.g. for scripting/testing)
  socratix ask [flags] "plan or question"
                              One-shot council on a single API key: a few personas on one
                              model stress-test your plan and print a decision memo. See
                              "socratix ask --list" for modes (pre-mortem, red team, tenth man).
  socratix consensus <transcript.json> [flags]
                              Evaluate consensus on a debate transcript; derives agents from
                              the transcript if --config is omitted.
  socratix service install      Register the engine to start at login (launchd on macOS,
                              systemd --user on Linux; not yet supported on Windows)
  socratix service uninstall    Remove that login-time registration
  socratix service status       Report whether it's currently registered

serve flags:
  --host string             Address to bind (default "127.0.0.1:7890", or env DIALEX_HOST)
  --port int                Port to listen on (overrides port in --host, or env DIALEX_PORT)
  --allow-insecure-lan      Required to bind to anything other than loopback (or env DIALEX_ALLOW_INSECURE_LAN)
  --tls                     Serve HTTPS (self-signed cert, generated once, if none given, or env DIALEX_TLS)
  --tls-cert string         Existing certificate file (with --tls-key, or env DIALEX_TLS_CERT)
  --tls-key string          Existing key file (with --tls-cert, or env DIALEX_TLS_KEY)
  --dir string              Override the config directory (default: platform-standard, or env DIALEX_CONFIG_DIR)`)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	hostFlag := fs.String("host", "", "address to bind (default 127.0.0.1:7890 or env DIALEX_HOST)")
	portFlag := fs.Int("port", 0, "port to listen on (default 7890 or env DIALEX_PORT)")
	allowInsecureLANFlag := fs.Bool("allow-insecure-lan", false, "required to bind to a non-loopback address (or env DIALEX_ALLOW_INSECURE_LAN)")
	useTLSFlag := fs.Bool("tls", false, "serve HTTPS (or env DIALEX_TLS)")
	tlsCertFlag := fs.String("tls-cert", "", "existing certificate file (or env DIALEX_TLS_CERT)")
	tlsKeyFlag := fs.String("tls-key", "", "existing key file (or env DIALEX_TLS_KEY)")
	dirFlag := fs.String("dir", "", "override the config directory (or env DIALEX_CONFIG_DIR)")

	// Embedded Tailscale flags
	tsnetFlag := fs.Bool("tsnet", false, "enable embedded userspace Tailscale node (or env DIALEX_TSNET)")
	tsnetAuthKeyFlag := fs.String("tsnet-authkey", "", "Tailscale auth key (or env DIALEX_TSNET_AUTHKEY / TS_AUTHKEY)")
	tsnetHostnameFlag := fs.String("tsnet-hostname", "", "Tailscale hostname (or env DIALEX_TSNET_HOSTNAME)")
	tsnetEphemeralFlag := fs.Bool("tsnet-ephemeral", false, "ephemeral Tailscale node (or env DIALEX_TSNET_EPHEMERAL)")
	fs.Parse(args)

	// Environment variable fallbacks
	envHost := os.Getenv("DIALEX_HOST")
	envPort := os.Getenv("DIALEX_PORT")
	envAllowLAN := os.Getenv("DIALEX_ALLOW_INSECURE_LAN")
	envTLS := os.Getenv("DIALEX_TLS")
	envCert := os.Getenv("DIALEX_TLS_CERT")
	envKey := os.Getenv("DIALEX_TLS_KEY")
	envDir := os.Getenv("DIALEX_CONFIG_DIR")

	envTsnet := os.Getenv("DIALEX_TSNET")
	envTsnetAuthKey := os.Getenv("DIALEX_TSNET_AUTHKEY")
	if envTsnetAuthKey == "" {
		envTsnetAuthKey = os.Getenv("TS_AUTHKEY")
	}
	envTsnetHostname := os.Getenv("DIALEX_TSNET_HOSTNAME")
	envTsnetEphemeral := os.Getenv("DIALEX_TSNET_EPHEMERAL")

	// Precedence: flag > env > default
	bindAddr := resolveBindAddr(*hostFlag, *portFlag, envHost, envPort)

	allowInsecureLAN := *allowInsecureLANFlag || isTruthy(envAllowLAN)
	useTLS := *useTLSFlag || isTruthy(envTLS)

	tlsCert := *tlsCertFlag
	if tlsCert == "" {
		tlsCert = envCert
	}
	tlsKey := *tlsKeyFlag
	if tlsKey == "" {
		tlsKey = envKey
	}

	configDir := *dirFlag
	if configDir == "" {
		configDir = envDir
	}
	if configDir == "" {
		var err error
		configDir, err = store.DefaultConfigDir()
		if err != nil {
			log.Fatalf("could not determine config directory: %v", err)
		}
	}

	st, err := store.New(configDir)
	if err != nil {
		log.Fatalf("could not open store at %s: %v", configDir, err)
	}

	// Capture server logs into ring buffer for Web UI live logs
	log.SetOutput(api.MultiLogWriter(os.Stderr, api.GlobalLogBuffer))

	server := api.NewServer(st)

	// Start embedded Tailscale node if requested
	useTsnet := *tsnetFlag || isTruthy(envTsnet)
	if useTsnet {
		tsHostname := *tsnetHostnameFlag
		if tsHostname == "" {
			tsHostname = envTsnetHostname
		}
		if tsHostname == "" {
			tsHostname = "dialex-server"
		}

		tsAuthKey := *tsnetAuthKeyFlag
		if tsAuthKey == "" {
			tsAuthKey = envTsnetAuthKey
		}

		tsEphemeral := *tsnetEphemeralFlag || isTruthy(envTsnetEphemeral)

		tsCfg := api.TsnetConfig{
			Enabled:   true,
			Hostname:  tsHostname,
			AuthKey:   tsAuthKey,
			StateDir:  configDir + "/tsnet",
			Ephemeral: tsEphemeral,
			Port:      7890,
		}

		_, _, err := api.StartTsnetServer(nil, tsCfg, server.Router(), api.GlobalLogBuffer)
		if err != nil {
			log.Printf("Warning: failed to start tsnet server: %v", err)
		}
	}

	log.Printf("Dialex engine listening on %s (config: %s)", bindAddr, configDir)
	err = server.ListenAndServe(bindAddr, api.ServeOptions{
		AllowInsecureLAN: allowInsecureLAN,
		UseTLS:           useTLS,
		TLSCertFile:      tlsCert,
		TLSKeyFile:       tlsKey,
		CertDir:          configDir,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func isTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func resolveBindAddr(flagHost string, flagPort int, envHost, envPort string) string {
	rawHost := flagHost
	if rawHost == "" {
		rawHost = envHost
	}
	if rawHost == "" {
		rawHost = "127.0.0.1:7890"
	}

	// Separate host and port if rawHost contains a colon
	h, p, err := net.SplitHostPort(rawHost)
	if err != nil {
		// No port in rawHost
		h = rawHost
		p = "7890"
	}

	// Override port if specified
	if flagPort > 0 {
		p = fmt.Sprintf("%d", flagPort)
	} else if envPort != "" {
		p = envPort
	}

	return net.JoinHostPort(h, p)
}

func runUsers(args []string) {
	if len(args) < 1 || args[0] != "add" {
		fmt.Fprintln(os.Stderr, "usage: roundtable users add [--dir path] <username>")
		os.Exit(1)
	}
	fs := flag.NewFlagSet("users add", flag.ExitOnError)
	dir := fs.String("dir", "", "override the config directory (same store `serve --dir` uses)")
	fs.Parse(args[1:])
	positional := fs.Args()
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: roundtable users add [--dir path] <username>")
		os.Exit(1)
	}
	username := positional[0]

	configDir := *dir
	if configDir == "" {
		var err error
		configDir, err = store.DefaultConfigDir()
		if err != nil {
			log.Fatalf("could not determine config directory: %v", err)
		}
	}
	st, err := store.New(configDir)
	if err != nil {
		log.Fatalf("could not open store at %s: %v", configDir, err)
	}
	fmt.Print("Password: ")
	var password string
	if _, err := fmt.Scanln(&password); err != nil {
		log.Fatalf("could not read password: %v", err)
	}
	if _, err := api.CreateFirstUser(st, username, password); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Added user %q.\n", username)
}

func runService(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: roundtable service <install|uninstall|status> [--host addr] [--dir path]")
		os.Exit(1)
	}
	fs := flag.NewFlagSet("service "+args[0], flag.ExitOnError)
	host := fs.String("host", "", "address to bind (passed through to serve; default 127.0.0.1:7890)")
	dir := fs.String("dir", "", "override the config directory (passed through to serve)")
	fs.Parse(args[1:])

	switch args[0] {
	case "install":
		if err := service.Install(service.Options{Host: *host, ConfigDir: *dir}); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Installed. The engine will now start automatically at login.")
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Uninstalled.")
	case "status":
		installed, err := service.Status()
		if err != nil {
			log.Fatal(err)
		}
		if installed {
			fmt.Println("installed")
		} else {
			fmt.Println("not installed")
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: roundtable service <install|uninstall|status> [--host addr] [--dir path]")
		os.Exit(1)
	}
}
