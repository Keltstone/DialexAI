plugins {
    alias(libs.plugins.androidApplication)
    alias(libs.plugins.kotlinMultiplatform)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.composeCompiler)
}

kotlin {
    androidTarget()
    sourceSets {
        val androidMain by getting {
            dependencies {
                implementation(project(":shared"))
                implementation(compose.material3)
                implementation(compose.ui)
                implementation(libs.kotlinx.coroutines.android)
                implementation("androidx.activity:activity-compose:1.9.3")
                implementation("androidx.biometric:biometric:1.1.0")
            }
        }
    }
}

android {
    namespace = "com.dialex.android"
    compileSdk = libs.versions.android.compileSdk.get().toInt()
    defaultConfig {
        applicationId = "com.keltstone.dialex"
        minSdk = libs.versions.android.minSdk.get().toInt()
        targetSdk = libs.versions.android.targetSdk.get().toInt()
        versionCode = (project.findProperty("app.versionCode") as? String)?.toIntOrNull() ?: 1
        versionName = project.findProperty("app.version") as? String ?: "1.0.0"
        // The engine binary is bundled as a "native library" (see src/androidMain/jniLibs/
        // arm64-v8a/libroundtable.so — it's a real executable, not a .so; naming it that way
        // is what makes Android's installer extract it with the execute bit set, the
        // standard trick for shipping an arbitrary binary with an APK) — only built for
        // arm64-v8a so far, real devices, not the x86_64 emulator.
        ndk { abiFilters += "arm64-v8a" }
    }
    buildFeatures { compose = true }
    // Must agree with the manifest's android:extractNativeLibs="true" — otherwise AGP
    // packages libroundtable.so compressed/page-aligned for mmap-only use, and it never
    // lands on disk as a runnable file for ProcessBuilder to exec.
    packaging { jniLibs { useLegacyPackaging = true } }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
}
