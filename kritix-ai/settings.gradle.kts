rootProject.name = "Kritix"

pluginManagement {
    includeBuild("${settings.providers.gradleProperty("koltLibsDir").get()}/build-logic")
    repositories {
        google()
        gradlePluginPortal()
        mavenCentral()
        mavenLocal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)
    repositories {
        google()
        mavenCentral()
        mavenLocal()
    }
}

val koltLibsDir = providers.gradleProperty("koltLibsDir").get()

include(":app")

// chained builds first, Kolt last (hard rule, see kolt-libs.md)
apply(from = "$koltLibsDir/gradle/kolt-consumer.settings.gradle.kts")
