import org.jetbrains.compose.desktop.application.dsl.TargetFormat

plugins {
    alias(libs.plugins.kotlinMultiplatform)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.composeCompiler)
    alias(libs.plugins.kotlinSerialization)
}

kotlin {
    jvm()
    sourceSets {
        jvmMain.dependencies {
            implementation(compose.desktop.currentOs)
            implementation(compose.material3)
            implementation(compose.materialIconsExtended)
            implementation(compose.foundation)
            implementation(compose.ui)
            implementation(libs.kotlinx.coroutines.swing)
            implementation(libs.kotlinx.serialization.json)
            implementation(libs.kotlinx.collections.immutable)
            implementation(libs.lifecycle.viewmodel)
            implementation(libs.lifecycle.viewmodel.compose)
            implementation(libs.lifecycle.runtime.compose)
            implementation("io.github.koltalabs.kolt:utils")
            implementation("io.github.koltalabs.kolt:logutils")
            implementation("io.github.koltalabs.kolt:compose-kmp")
        }
    }
}

compose.desktop {
    application {
        mainClass = "com.artix.desktop.MainKt"
        jvmArgs += listOf(
            "-Xdock:name=Artix AI",
            "-Dapple.awt.application.name=Artix AI",
            "-Dapple.awt.application.appearance=system",
            "-Dapple.laf.useScreenMenuBar=true"
        )
        nativeDistributions {
            targetFormats(TargetFormat.Dmg, TargetFormat.Msi, TargetFormat.Exe, TargetFormat.Deb, TargetFormat.Rpm)
            packageName = "ArtixAI"
            packageVersion = project.findProperty("app.version") as? String ?: "1.0.0"
            vendor = "Kolta Labs"
            description = "Artix AI - Dialectic Software Engineering & Autonomous Coding Cockpit"
            val engineResourcesDir = project.findProperty("engine.resources.dir") as? String
            if (!engineResourcesDir.isNullOrBlank()) {
                appResourcesRootDir.set(project.file(engineResourcesDir))
            }
            macOS {
                bundleID = "com.koltalabs.artix"
                dockName = "Artix AI"
            }
            windows {
                menuGroup = "Kolta Labs"
                upgradeUuid = "D38E3120-1A8E-4A7B-8B33-25A932E49E01"
                shortcut = true
                menu = true
            }
            linux {
                packageName = "artix-ai"
            }
        }
    }
}
