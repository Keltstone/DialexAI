import org.jetbrains.compose.desktop.application.dsl.TargetFormat

plugins {
    alias(libs.plugins.kotlinMultiplatform)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.composeCompiler)
}

kotlin {
    jvm()
    sourceSets {
        jvmMain.dependencies {
            implementation(project(":shared"))
            implementation(compose.desktop.currentOs)
            implementation(compose.material3)
            implementation(compose.materialIconsExtended)
            implementation(libs.kotlinx.coroutines.swing)
            implementation(libs.kotlinx.serialization.json)
            implementation(libs.lifecycle.viewmodel)
            implementation(libs.lifecycle.viewmodel.compose)
            implementation(libs.lifecycle.runtime.compose)
            implementation("io.github.koltalabs.kolt:logutils")
        }
    }
}

compose.desktop {
    application {
        mainClass = "com.dialex.desktop.MainKt"
        jvmArgs += listOf(
            "-Xdock:name=Dialex",
            "-Dapple.awt.application.name=Dialex",
            "-Dapple.awt.application.appearance=system",
            "-Dapple.laf.useScreenMenuBar=true"
        )
        nativeDistributions {
            targetFormats(TargetFormat.Dmg, TargetFormat.Msi, TargetFormat.Exe, TargetFormat.Deb, TargetFormat.Rpm)
            packageName = "DialexAI"
            packageVersion = project.findProperty("app.version") as? String ?: "1.0.0"
            vendor = "Kolta Labs"
            description = "Dialex AI by Kolta Labs - Sovereign Multi-Agent AI Deliberation Platform"
            val engineResourcesDir = project.findProperty("engine.resources.dir") as? String
            if (!engineResourcesDir.isNullOrBlank()) {
                appResourcesRootDir.set(project.file(engineResourcesDir))
            }
            macOS {
                bundleID = "com.koltalabs.dialex"
                dockName = "Dialex AI"
                iconFile.set(project.file("src/jvmMain/resources/icon.icns"))
            }
            windows {
                menuGroup = "Kolta Labs"
                upgradeUuid = "B9F01447-2442-4818-A9D3-7BD6718C80DE"
                shortcut = true
                menu = true
                iconFile.set(project.file("src/jvmMain/resources/icon.ico"))
            }
            linux {
                iconFile.set(project.file("src/jvmMain/resources/icon.png"))
            }
        }
    }
}

tasks.withType<JavaExec> {
    jvmArgs(
        "-Xdock:name=Dialex",
        "-Dapple.awt.application.name=Dialex",
        "-Dapple.awt.application.appearance=system",
        "-Dapple.laf.useScreenMenuBar=true"
    )
}

tasks.register<JavaExec>("runPersonaStudio") {
    group = "application"
    description = "Runs the standalone Dialex Persona Studio application"
    val jvmMainCompilation = kotlin.targets.getByName("jvm").compilations.getByName("main")
    classpath = files(
        jvmMainCompilation.output.allOutputs,
        jvmMainCompilation.runtimeDependencyFiles
    )
    mainClass.set("com.dialex.desktop.personastudio.PersonaStudioMainKt")
    jvmArgs(
        "-Xdock:name=Persona Studio",
        "-Dapple.awt.application.name=Persona Studio",
        "-Dapple.awt.application.appearance=system",
        "-Dapple.laf.useScreenMenuBar=true"
    )
}

