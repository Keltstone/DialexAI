# Dialex AI Client: Developer Guide & Build Pipeline

**Client:** `dialex-ai` (Kotlin Multiplatform / Compose Multiplatform)  
**Author:** Kolta Labs Core Mobile & Desktop Architecture  
**Audience:** Mobile Engineers, Desktop Developers, KMP Contributors  
**Scope:** Gradle multi-project setup, Kolt composite builds, building native distributables, and writing tests.

---

## 1. Development Workspace & Prerequisites

### 1.1 Toolchain Requirements
- **JDK 21+** (Azul Zulu, Eclipse Temurin, or Amazon Corretto).
- **Android SDK 35+** (for Android client compilation).
- **Xcode 16+** (for macOS native packaging).
- **Gradle 8.10+** (managed via `./gradlew` wrapper).

---

## 2. The Kolt Ecosystem Composite Build (`includeBuild`)

> [!IMPORTANT]
> **Dialex AI depends directly on the Kolt Ecosystem (`Kolt`) via a Gradle Composite Build (`includeBuild`).**  
> Kolt is always an `includeBuild` in this workspace.

### Directory Structure Requirement
Clone `Kolt` into the same parent folder alongside `DialexAI`:

```bash
mkdir -p ~/Workspace && cd ~/Workspace
git clone https://github.com/Kolta-Labs/Kolt.git
git clone https://github.com/Kolta-Labs/DialexAI.git

# Verify layout:
# ~/Workspace/
#   ├── Kolt/
#   └── DialexAI/
```

In `dialex-ai/settings.gradle.kts`:
```kotlin
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
    }
}

// Composite build binding to Kolt
val koltDir = file("../Kolt")
if (koltDir.exists()) {
    includeBuild("../Kolt")
}
```

---

## 3. Building & Running the Applications

### 3.1 Desktop Application (Compose Desktop)

```bash
cd dialex-ai

# Run desktop app directly in development mode:
./gradlew :desktopApp:run

# Package native macOS DMG:
./gradlew :desktopApp:packageDmg

# Package Windows MSI installer:
./gradlew :desktopApp:packageMsi

# Package Linux Deb / AppImage:
./gradlew :desktopApp:packageDeb
./gradlew :desktopApp:packageAppImage
```

Distributables are generated in: `dialex-ai/desktopApp/build/compose/binaries/main/`.

### 3.2 Android Application (Compose Android)

```bash
cd dialex-ai

# Assemble Debug APK:
./gradlew :androidApp:assembleDebug

# Install directly to connected USB device or emulator:
./gradlew :androidApp:installDebug

# Assemble Signed Release APK:
./gradlew :androidApp:assembleRelease
```

Output APK: `dialex-ai/androidApp/build/outputs/apk/debug/androidApp-debug.apk`.

---

## 4. How to Create a New MVI Screen

To add a new screen (e.g. `presentation/telemetry/`), adhere strictly to **Engine Rule 4**:

### Step 1: Create Package Directory
Create `shared/src/commonMain/kotlin/com/dialex/presentation/telemetry/` containing:
- `Contract.kt`
- `TelemetryViewModel.kt`
- `TelemetryRoute.kt`
- `TelemetryScreen.kt`

### Step 2: Define `Contract.kt`
```kotlin
package com.dialex.presentation.telemetry

import androidx.compose.runtime.Immutable
import kotlinx.collections.immutable.ImmutableList
import kotlinx.collections.immutable.persistentListOf

interface TelemetryContract {
    @Immutable
    data class State(
        val activeDebatesCount: Int = 0,
        val logs: ImmutableList<String> = persistentListOf(),
        val isHealthy: Boolean = true
    )

    sealed interface Intent {
        data object Refresh : Intent
        data object ClearLogs : Intent
    }

    sealed interface Effect {
        data class ShowSnackbar(val text: String) : Effect
    }
}
```

### Step 3: Implement `TelemetryViewModel.kt`
```kotlin
package com.dialex.presentation.telemetry

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch

class TelemetryViewModel : ViewModel() {
    private val _state = MutableStateFlow(TelemetryContract.State())
    val state: StateFlow<TelemetryContract.State> = _state.asStateFlow()

    private val _effect = Channel<TelemetryContract.Effect>(Channel.BUFFERED)
    val effect: Flow<TelemetryContract.Effect> = _effect.receiveAsFlow()

    fun onIntent(intent: TelemetryContract.Intent) {
        when (intent) {
            is TelemetryContract.Intent.Refresh -> loadTelemetry()
            is TelemetryContract.Intent.ClearLogs -> clear()
        }
    }

    private fun loadTelemetry() {
        viewModelScope.launch {
            // Call domain UseCase
        }
    }
    
    private fun clear() {
        _state.update { it.copy(logs = persistentListOf()) }
    }
}
```

### Step 4: Implement `TelemetryRoute.kt` and `TelemetryScreen.kt`
```kotlin
@Composable
fun TelemetryRoute(
    viewModel: TelemetryViewModel = viewModel(),
    onNavigateBack: () -> Unit
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    
    LaunchedEffect(Unit) {
        viewModel.effect.collect { effect ->
            when (effect) {
                is TelemetryContract.Effect.ShowSnackbar -> { /* Show snackbar */ }
            }
        }
    }

    TelemetryScreen(
        state = state,
        onIntent = viewModel::onIntent,
        onNavigateBack = onNavigateBack
    )
}

@Composable
fun TelemetryScreen(
    state: TelemetryContract.State,
    onIntent: (TelemetryContract.Intent) -> Unit,
    onNavigateBack: () -> Unit
) {
    // Stateless UI using Kolt tokens
}
```

---

## 5. Running Verification & Konsist Architecture Tests

Dialex AI enforces Konsist architecture and Detekt lint tests:

```bash
cd dialex-ai

# Run all unit tests across commonMain:
./gradlew :shared:allTests

# Run architecture guardrails & Konsist checks:
./gradlew check
```

---

*Next: Continue to [Artix AI Platform: Conceptual Foundation](../artix-ai/01_CONCEPTUAL_FOUNDATION.md).*
