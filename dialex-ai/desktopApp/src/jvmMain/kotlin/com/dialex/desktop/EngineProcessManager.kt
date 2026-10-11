package com.dialex.desktop

import com.dialex.engine.EngineClient
import com.dialex.engine.EngineException
import kotlinx.coroutines.delay
import java.io.File
import java.security.SecureRandom
import java.util.Base64

/**
 * Makes the desktop app engine-backed without the user ever seeing "local vs. engine" as a
 * choice: on launch, reuse an already-running engine on the default port (e.g. one installed
 * as a login service via `roundtable service install`, or another instance of this app), or
 * spawn one itself and manage its lifecycle. Either way, hands back a logged-in [EngineClient].
 *
 * The account used here is desktop-private (a random password generated once and stored
 * loopback-side in the engine's own config dir, 0600) — there's no login screen for this path.
 * A user connecting to a *different*, self-hosted engine instead is a separate, not-yet-built
 * flow (real username/password entered by hand); this manager only covers "run my own".
 */
object EngineProcessManager {
    private const val port = 7890
    private val baseUrl = "http://127.0.0.1:$port"
    private const val desktopUsername = "desktop"

    class BootstrapException(message: String, cause: Throwable? = null) : Exception(message, cause)

    private var spawnedProcess: Process? = null

    suspend fun start(): EngineClient {
        val configDir = configDir()
        configDir.mkdirs()

        if (!isHealthy()) {
            val binary = resolveEngineBinary()
                ?: throw BootstrapException(
                    "Could not find the dialex engine binary. Build it with " +
                        "`go build -o engine/build/dialex ./cmd/dialex` from the engine/ " +
                        "directory (dev), or install it on PATH."
                )
            val pb = ProcessBuilder(binary.absolutePath, "serve", "--host", "127.0.0.1:$port", "--dir", configDir.absolutePath)
            pb.environment()["PATH"] = com.dialex.runner.resolvedShellPath
            spawnedProcess = pb
                .redirectOutput(File(configDir, "engine.log"))
                .redirectErrorStream(true)
                .start()
            Runtime.getRuntime().addShutdownHook(Thread { spawnedProcess?.destroy() })
            waitForHealth()
        }

        val password = loadOrCreatePassword(configDir)
        val client = EngineClient(baseUrl)
        try {
            client.login(desktopUsername, password)
        } catch (e: EngineException) {
            if (e.status != 401) throw e
            // First run against this store: no "desktop" account yet — create it with the
            // password we just generated, then log in for real.
            val binary = resolveEngineBinary() ?: throw BootstrapException("Engine is running but its binary can't be found to seed the desktop account.")
            createDesktopUser(binary, configDir, password)
            client.login(desktopUsername, password)
        }
        return client
    }

    fun stop() {
        spawnedProcess?.destroy()
        spawnedProcess = null
    }

    /** The OS PID of the engine subprocess *this app instance* spawned, if it spawned one at
     * all (reusing an already-running engine — a service install, or another app instance —
     * leaves this null). Used only to label that one row in Settings → Servers as "this app",
     * not to distinguish what's safe to kill — every listed process is equally killable. */
    fun spawnedPid(): Long? = spawnedProcess?.pid()

    private suspend fun isHealthy(): Boolean = runCatching { EngineClient(baseUrl).health() }.isSuccess

    private suspend fun waitForHealth() {
        val deadline = System.currentTimeMillis() + 10_000
        while (System.currentTimeMillis() < deadline) {
            if (isHealthy()) return
            delay(150)
        }
        throw BootstrapException("Engine did not become healthy on $baseUrl within 10s — check ${File(configDir(), "engine.log")}")
    }

    private fun createDesktopUser(binary: File, configDir: File, password: String) {
        val process = ProcessBuilder(binary.absolutePath, "users", "add", "--dir", configDir.absolutePath, desktopUsername)
            .redirectErrorStream(true)
            .start()
        process.outputStream.bufferedWriter().use { it.write("$password\n") }
        val log = process.inputStream.bufferedReader().readText()
        if (process.waitFor() != 0) throw BootstrapException("Could not create the desktop account:\n$log")
    }

    private fun loadOrCreatePassword(configDir: File): String {
        val file = File(configDir, ".desktop-credentials")
        if (file.exists()) return file.readText().trim()
        val bytes = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val password = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
        file.writeText(password)
        file.setReadable(false, false)
        file.setReadable(true, true)
        file.setWritable(false, false)
        file.setWritable(true, true)
        return password
    }

    /** Mirrors `store.DefaultConfigDir()` on the Go side exactly — desktop and the engine it
     * spawns must agree on where the store (and this credentials file) live. Also where
     * [ConnectionPreferenceStore] keeps its own file — same directory, different concern. */
    fun configDir(): File {
        val home = System.getProperty("user.home")
        val os = System.getProperty("os.name").lowercase()
        return when {
            os.contains("mac") -> File(home, "Library/Application Support/Dialex")
            os.contains("win") -> File(System.getenv("APPDATA") ?: File(home, "AppData/Roaming").path, "Dialex")
            else -> File(System.getenv("XDG_CONFIG_HOME") ?: File(home, ".config").path, "dialex")
        }
    }

    /** Resolves the engine binary: checks bundled app resources first, then dev checkout, then PATH & standard locations. */
    private fun resolveEngineBinary(): File? {
        // 1. Compose application resources directory (when packaged with engine)
        val resourcesDir = System.getProperty("compose.application.resources.dir")
        if (!resourcesDir.isNullOrBlank()) {
            val bundled = listOf(
                File(resourcesDir, "engine/socratix"),
                File(resourcesDir, "engine/socratix.exe"),
                File(resourcesDir, "socratix"),
                File(resourcesDir, "socratix.exe")
            )
            bundled.firstOrNull { it.exists() && it.canExecute() }?.let { return it }
        }

        // 2. Relative to macOS .app bundle Contents
        val userDir = System.getProperty("user.dir") ?: ""
        val macAppEngine = File(userDir, "../Resources/engine/socratix")
        if (macAppEngine.exists() && macAppEngine.canExecute()) return macAppEngine

        // 3. Dev checkout paths
        val userDirFile = File(userDir)
        val candidates = listOf(
            File(userDirFile.parentFile, "socratix-engine/bin/socratix"),
            File(userDirFile.parentFile, "socratix-engine/bin/socratix.exe"),
            File(userDirFile.parentFile, "engine/build/dialex"),
            File(userDirFile.parentFile, "engine/build/roundtable"),
            File(userDirFile.parentFile, "socratix-engine/build/dialex"),
        )
        candidates.firstOrNull { it.exists() && it.canExecute() }?.let { return it }

        // 4. PATH lookup + standard platform paths
        val standardLocations = listOf(
            "/usr/local/bin",
            "/opt/homebrew/bin",
            System.getProperty("user.home") + "/.local/bin"
        )
        val pathDirs = (System.getenv("PATH")?.split(File.pathSeparator).orEmpty() + standardLocations).distinct()
        for (name in listOf("socratix", "dialex", "roundtable", "socratix.exe", "dialex.exe")) {
            val found = pathDirs.map { File(it, name) }.firstOrNull { it.exists() && it.canExecute() }
            if (found != null) return found
        }
        return null
    }
}
