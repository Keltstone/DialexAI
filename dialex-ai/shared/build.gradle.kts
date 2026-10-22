plugins {
    alias(libs.plugins.kotlinMultiplatform)
    alias(libs.plugins.kotlinSerialization)
    alias(libs.plugins.androidLibrary)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.composeCompiler)
}

kotlin {
    compilerOptions {
        freeCompilerArgs.add("-Xexpect-actual-classes")
    }
    jvm("desktop")
    androidTarget()

    sourceSets {
        commonMain.dependencies {
            implementation(libs.kotlinx.coroutines.core)
            implementation(libs.kotlinx.serialization.json)
            implementation(libs.kotlinx.collections.immutable)
            // Kolt Library Inclusion
            implementation("io.github.keltstone.kolt:utils")
            implementation("io.github.keltstone.kolt:logutils")
            implementation("io.github.keltstone.kolt:compose-kmp")
            // api: EngineClient takes an HttpClient, so callers need this type too.
            api(libs.ktor.client.core)
            implementation(libs.ktor.client.content.negotiation)
            implementation(libs.ktor.serialization.kotlinx.json)
            // KMP ViewModel + lifecycle (stable for commonMain since Lifecycle 2.8+).
            implementation(libs.lifecycle.viewmodel)
            implementation(libs.lifecycle.viewmodel.compose)
            implementation(libs.lifecycle.runtime.compose)
            // UI lives here too: desktopApp/androidApp are thin shells, all screens shared.
            implementation(compose.runtime)
            implementation(compose.foundation)
            implementation(compose.ui)
            implementation(compose.material3)
            implementation(compose.materialIconsExtended)
            implementation(compose.components.uiToolingPreview)
            implementation(compose.components.resources)
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
        }
        val desktopTest by getting {
            dependencies {
                // Architecture rules from CLAUDE.md, checked as ordinary tests (JVM-only tool).
                implementation("com.lemonappdev:konsist:0.17.3")
            }
        }
        val desktopMain by getting {
            dependencies {
                implementation(libs.ktor.client.cio)
                implementation(libs.kotlinx.coroutines.swing)
                implementation("org.xerial:sqlite-jdbc:3.47.1.0")
            }
        }
        val androidMain by getting {
            dependencies {
                implementation(libs.ktor.client.okhttp)
                implementation(libs.kotlinx.coroutines.android)
                implementation("androidx.activity:activity-compose:1.9.3")
            }
        }
    }
}

android {
    namespace = "com.dialex.shared"
    compileSdk = libs.versions.android.compileSdk.get().toInt()
    defaultConfig {
        minSdk = libs.versions.android.minSdk.get().toInt()
    }
    // Without this, javac defaults to 1.8 while Kotlin's androidTarget compiles against
    // whatever JDK is running Gradle (21 on this machine, no toolchain override) —
    // "Inconsistent JVM-target compatibility". Matching javac to that avoids provisioning a
    // second JDK via a Kotlin toolchain just for this.
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
}
