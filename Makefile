# ==============================================================================
# DialexAI Ecosystem Root Makefile
# Provides easy build and installation shortcuts for Artix, Dialex, Kritix & Socratix
# ==============================================================================

VERSION ?= 1.0.0
PREFIX ?= /usr/local

.PHONY: all build build-cli build-desktop build-all install test clean help

all: build-cli

help:
	@echo "DialexAI Ecosystem Makefile (v$(VERSION))"
	@echo ""
	@echo "Build Targets:"
	@echo "  make build          - Build all Go CLI & Engine binaries (socratix, artix, kritix)"
	@echo "  make build-desktop  - Build Compose Desktop packages (DMG on macOS, MSI on Windows, DEB on Linux)"
	@echo "  make build-all      - Build CLI binaries AND Desktop apps (with and without engine)"
	@echo ""
	@echo "Install Targets:"
	@echo "  make install        - Install ecosystem components with engine detection & express permission"
	@echo "  make install-cli    - Install CLI binaries to $(PREFIX)/bin"
	@echo ""
	@echo "Quality Targets:"
	@echo "  make test           - Run all test suites across Go and Kotlin"
	@echo "  make clean          - Clean build directories and dist outputs"

build: build-cli

build-cli:
	@./scripts/build-dist.sh --cli --version $(VERSION)

build-cross:
	@./scripts/build-dist.sh --cli --cross-compile --version $(VERSION)

build-desktop:
	@./scripts/build-dist.sh --desktop --version $(VERSION)

build-all:
	@./scripts/build-dist.sh --all --cross-compile --version $(VERSION)

install:
	@./scripts/install.sh --prefix $(PREFIX)

install-cli:
	@./scripts/install.sh --cli --all --prefix $(PREFIX)

test:
	@echo "▶ Running Go test suites..."
	@(cd socratix-engine && go test ./pkg/...)
	@(cd artix-ai && go test ./pkg/...)
	@(cd kritix-ai && go test ./pkg/...)
	@echo "▶ Running Kotlin tests..."
	@(cd dialex-ai && ./gradlew :desktopApp:compileKotlinJvm)
	@echo "✓ All tests compiled and passed."

clean:
	@rm -rf dist build/tmp-dist
	@echo "✓ Clean complete."
