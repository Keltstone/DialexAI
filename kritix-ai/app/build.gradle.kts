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
            implementation("io.github.keltstone.kolt:utils")
            implementation("io.github.keltstone.kolt:logutils")
            implementation("io.github.keltstone.kolt:compose-kmp")
        }
    }
}

compose.desktop {
    application {
        mainClass = "com.kritix.desktop.MainKt"
        jvmArgs += listOf(
            "-Xdock:name=Kritix AI",
            "-Dapple.awt.application.name=Kritix AI",
            "-Dapple.awt.application.appearance=system",
            "-Dapple.laf.useScreenMenuBar=true"
        )
        nativeDistributions {
            targetFormats(TargetFormat.Dmg, TargetFormat.Msi, TargetFormat.Exe, TargetFormat.Deb, TargetFormat.Rpm)
            packageName = "KritixAI"
            packageVersion = project.findProperty("app.version") as? String ?: "1.0.0"
            vendor = "Kolta Labs"
            description = "Kritix AI — Autonomous QA Testing Studio & Cockpit"
            val engineResourcesDir = project.findProperty("engine.resources.dir") as? String
            if (!engineResourcesDir.isNullOrBlank()) {
                appResourcesRootDir.set(project.file(engineResourcesDir))
            }
            macOS {
                bundleID = "com.koltalabs.kritix"
                dockName = "Kritix AI"
            }
            windows {
                menuGroup = "Kolta Labs"
                upgradeUuid = "E49F4231-2B9F-4C8C-9C44-36BA43F50F12"
                shortcut = true
                menu = true
            }
            linux {
                packageName = "kritix-ai"
            }
        }
    }
}
