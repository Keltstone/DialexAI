# Dialex AI Developer Guide & Architecture Standards

This guide is intended for developers contributing to, extending, or building **Dialex AI** from source. It details the project structure, development workflow, testing methodologies, architectural invariants, CI/CD pipelines, release automation, and an in-depth guide to the **Kolt Ecosystem (`KoltLibs`)** dependency.

---

## 1. Codebase Structure & Module Layout

Dialex AI is organized as a multi-target Gradle project paired with a high-throughput Go backend:

```
DialexAI/
├── shared/                         # Core Kotlin Multiplatform (KMP) Module
│   ├── src/
│   │   ├── commonMain/kotlin/com/dialex/
│   │   │   ├── domain/             # Pure Kotlin Domain (UseCases, Repositories, Entities)
│   │   │   ├── data/               # Ktor HTTP Client, SSE Parser, AppLogStore
│   │   │   ├── model/              # Domain Models (Discussion, Agent, Persona, Deliverable)
│   │   │   ├── orchestrator/       # Client-side cost/deliberation estimates (the Go engine is the only orchestrator)
│   │   │   ├── presentation/       # MVI Screens (Chat, Setup, Settings, Workspace)
│   │   │   ├── theme/              # CcPalette, LocalCcColors, Typography Tokens
│   │   │   └── ui/                 # Reusable Compose Multiplatform Components
│   │   ├── desktopMain/kotlin/     # Desktop-specific JVM bindings & SQLite
│   │   ├── androidMain/kotlin/     # Android-specific OkHttp client & Activity bindings
│   │   └── commonTest/kotlin/      # Platform-agnostic unit tests (kotlin.test)
├── desktopApp/                     # Thin Compose Desktop Shell (macOS, Linux, Windows)
│   └── src/jvmMain/kotlin/com/dialex/desktop/Main.kt
├── androidApp/                     # Thin Android Shell (Dialex AI Mobile)
│   └── src/androidMain/kotlin/com/dialex/android/MainActivity.kt
├── socratix-engine/                        # Go Orchestration Engine
│   ├── cmd/socratix/                 # CLI entry point (main.go)
│   └── pkg/
│       ├── api/                    # HTTP REST handlers & SSE streaming hub
│       ├── orchestrator/           # Deliberation turn loop & state machine
│       ├── runner/                 # Cloud API & local CLI agent execution runners
│       ├── store/                  # Atomic file persistence & AES-256 vault
│       └── tailscale/              # Embedded tsnet WireGuard integration
├── KoltLibs/                       # Kolt Composite Build & Conventions (sibling directory)
├── tools/                          # Developer Tooling & Version Bump Scripts
└── docs/                           # Comprehensive Technical Documentation Suite
```

---

## 2. Kolt Ecosystem (`KoltLibs`) Dependency & Setup

> [!IMPORTANT]
> **Dialex AI depends directly on the Kolt Ecosystem (`KoltLibs`) via a Gradle Composite Build (`includeBuild`).**  
> Anyone cloning, downloading, or forking the source code must also clone the `KoltLibs` repository into the same parent workspace directory.

### 2.1 Why KoltLibs is Required
Kolt provides the foundational architecture across all Kolta Labs multiplatform projects:
- **`io.github.keltstone.kolt:compose-kmp`**: The design system architecture (`CcPalette`, `LocalCcColors`, `ThemeMode`), elevation tokens, button styles, and fluid typography.
- **`io.github.keltstone.kolt:utils`**: The canonical `AsyncState<T>` reactive monad (`Uninitialized`, `Loading`, `Success`, `Error`) used across all ViewModels and domain repositories.
- **`io.github.keltstone.kolt:logutils`**: Release-gated multiplatform structured logging with level filters and telemetry sinks.

### 2.2 How to Obtain & Clone KoltLibs
To build Dialex AI from source, follow this workspace layout:

```bash
# 1. Create a parent workspace directory
mkdir -p ~/Workspace && cd ~/Workspace

# 2. Clone the Kolt repository
git clone https://github.com/keltstone/Kolt.git

# 3. Clone DialexAI alongside Kolt
git clone https://github.com/Kolta-Labs/DialexAI.git

# 4. Confirm the sibling directory layout:
# ~/Workspace/
#   ├── Kolt/ (or KoltLibs/)
#   └── DialexAI/
```

### 2.3 Gradle Composite Build Wiring
In `dialex-ai/settings.gradle.kts`, Gradle is configured to use the shared Kolt composite build script:

```kotlin
// gradle.properties
koltLibsDir=../../KoltLibs

// settings.gradle.kts
pluginManagement {
    includeBuild("${settings.providers.gradleProperty("koltLibsDir").get()}/build-logic")
    ...
}

val koltLibsDir = providers.gradleProperty("koltLibsDir").get()
...
// chained builds first, Kolt last (hard rule, see kolt-libs.md)
apply(from = "$koltLibsDir/gradle/kolt-consumer.settings.gradle.kts")
```

*Note: If your local development environment places Kolt in a different relative path, simply update `koltLibsDir` in `gradle.properties`.*

---

## 3. Clean Architecture & MVI Guidelines

Dialex AI strictly enforces Clean Architecture and unidirectional Model-View-Intent (MVI).

### 3.1 Layer Invariants (No Skipped Layers)
```
Presentation (ViewModel) ──► UseCase (Domain) ──► Repository Interface (Domain)
                                                         │
                                                         ▼
                                                RepositoryImpl (Data)
                                                         │
                                                         ▼
                                                DataSource (Data)
                                                         │
                                                         ▼
                                            EngineClient / Storage (Originator)
```

1. **Domain Layer has Zero Platform Dependencies:** No Android, JVM, Swing, or Compose imports in `domain/` or `model/`.
2. **Platform Exception Isolation:** Network exceptions (Ktor `HttpRequestException`), I/O errors, or JSON errors are caught at the `DataSource` boundary and translated into strongly typed `DomainException` objects (`DomainException.NetworkError`, `DomainException.Unauthorized`, `DomainException.RateLimited`).
3. **MVI Contract Structure:** Every screen contains a single `Contract.kt` defining:
   - `State`: Immutable data class. All collections must use `ImmutableList`, `ImmutableMap`, or `ImmutableSet` (`kotlinx.collections.immutable`).
   - `Intent`: Sealed interface representing all user interactions and lifecycle actions.
   - `Effect`: Sealed interface for one-shot UI events (navigation, snackbars, clipboard copies) consumed via a `Channel`.
4. **Stateless UI Composables:** The root screen composable accepts only `state: ScreenState`, `onIntent: (ScreenIntent) -> Unit`, and navigation callbacks. It never references a `ViewModel` directly, ensuring full `@Preview` support with mock data.

---

## 4. Development & Build Workflows

### 4.1 Prerequisites
- **Java Development Kit (JDK):** Version 17 or 21 LTS (Temurin, Azul Zulu, or OpenJDK).
- **Go:** Version 1.22+ (for compiling the backend engine).
- **Android SDK:** API Level 34+ (for Android builds).

### 4.2 Building & Running the Desktop App
```bash
# Run Compose Desktop application (spawns managed Go engine)
./gradlew :desktopApp:run

# Compile Desktop Kotlin binaries without running
./gradlew :shared:compileKotlinDesktop :desktopApp:compileKotlinJvm
```

### 4.3 Building & Running the Go Backend
```bash
cd socratix-engine

# Run all unit and race tests
go test -v -race ./pkg/...

# Run standalone development server
go run ./cmd/socratix/main.go --port 8080 --data-dir ~/.dialex
```

### 4.4 Running Test Suites
```bash
# Run KMP Common Unit Tests (pure Kotlin)
./gradlew :shared:allTests

# Run Go engine tests
cd engine && go test -race ./...
```

---

## 5. Adding New AI Providers or Deliverable Formats

### 5.1 Adding an AI Provider to the Go Engine
1. Implement the `runner.Runner` interface in `socratix-engine/pkg/runner/`:
   ```go
   type Runner interface {
       Generate(ctx context.Context, req *GenerationRequest) (*GenerationResponse, error)
       Stream(ctx context.Context, req *GenerationRequest, deltaCh chan<- string) (*GenerationResponse, error)
   }
   ```
2. Register the provider in `socratix-engine/pkg/runner/factory.go`.
3. Add the provider enum and icon mapping to `shared/src/commonMain/kotlin/com/dialex/model/Agent.kt`.

### 5.2 Adding a New Deliverable Format
1. Add the format enum to `shared/src/commonMain/kotlin/com/dialex/model/Deliverable.kt`:
   ```kotlin
   enum class DeliverableType(val displayName: String, val promptDirective: String) {
       EXECUTIVE_MEMO("Executive Memorandum", "Produce a formal executive briefing in HTML format..."),
       THREAT_MODEL("STRIDE Threat Model", "Generate a structured STRIDE threat analysis markdown table...")
   }
   ```
2. Add the action button trigger in `OutcomeCard.kt` and `DeliverablesPanel.kt`.

---

## 6. Versioning, CI/CD & GitHub Release Pipeline

Dialex AI uses an automated GitHub Actions release workflow that packages native executables across all target platforms, updates the `CHANGELOG.md`, and creates a GitHub Release.

### 6.1 Versioning Single Source of Truth
The version is controlled centrally in `gradle.properties`:
```properties
app.version=1.0.0
app.versionCode=1
```
All desktop, mobile, and engine packaging tasks consume `app.version` dynamically.

### 6.2 Bumping Versions Locally
Use the provided bump script:
```bash
# Bumps version, increments versionCode, updates CHANGELOG.md, commits, and tags git
./tools/bump-version.sh 1.1.0

# Dry run mode:
./tools/bump-version.sh 1.1.0 --dry-run
```

### 6.3 Triggering a Release via GitHub Actions
A release can be created in three ways:
1. **GitHub UI Workflow Dispatch**: Navigate to **Actions &gt; Release**, enter the new version (e.g. `1.1.0`), and click **Run workflow**.
2. **Version Bump Dispatch**: Navigate to **Actions &gt; Bump Version &amp; Tag Release**, enter the new version, and the workflow will update `gradle.properties`, update `CHANGELOG.md`, tag git, and trigger the build matrix automatically.
3. **Pushing a Git Tag**:
   ```bash
   git tag -a v1.1.0 -m "Release v1.1.0"
   git push origin v1.1.0
   ```

### 6.4 Release Artifact Matrix
The release pipeline automatically generates and publishes the following assets to GitHub Releases:
- **macOS**: `DialexAI-<version>-macOS-arm64.dmg`
- **Windows**: `DialexAI-<version>-Windows-x64.msi`
- **Linux**: `DialexAI-<version>-Linux-amd64.deb`, `DialexAI-<version>-Linux-amd64.rpm`
- **Android**: `DialexAI-Mobile-<version>.apk`
- **Go Engine Standalone Archives**:
  - `dialex-engine-v<version>-linux-amd64.tar.gz`
  - `dialex-engine-v<version>-linux-arm64.tar.gz`
  - `dialex-engine-v<version>-darwin-arm64.tar.gz`
  - `dialex-engine-v<version>-darwin-amd64.tar.gz`
  - `dialex-engine-v<version>-windows-amd64.zip`
- **Checksums**: `SHA256SUMS.txt`

---

## 7. Code Quality & Licensing Compliance

- All code must comply with **PolyForm Noncommercial License 1.0.0**.
- Retain license notices and author attributions across source files.
- Submit PRs with descriptive commit messages following the [Conventional Commits](https://www.conventionalcommits.org/) specification (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`).

---

*Copyright (c) 2026 Kolta Labs. Licensed under the [PolyForm Noncommercial License 1.0.0](file:///LICENSE).*
