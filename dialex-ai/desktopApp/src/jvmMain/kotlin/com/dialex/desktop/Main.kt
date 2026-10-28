package com.dialex.desktop

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.WindowPosition
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import java.awt.Dimension
import com.dialex.runner.checkCliAvailability
import com.dialex.runner.checkCliLoginStatus
import com.dialex.theme.AppTheme
import com.dialex.theme.ThemeMode
import com.dialex.ui.CliCatalog
import com.dialex.util.installCrashLogger
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import javax.swing.JFileChooser
import javax.swing.filechooser.FileNameExtensionFilter

/** Native "Save As" dialog, defaulting to the discussion's own name. Blocking (Swing modal)
 * is fine here — it's already a user-initiated click, and Compose Desktop shares the AWT
 * event thread anyway. */
private fun saveMarkdownAs(markdown: String, suggestedFileName: String) {
    val chooser = JFileChooser().apply {
        dialogTitle = "Export discussion"
        fileFilter = FileNameExtensionFilter("Markdown (*.md)", "md")
        selectedFile = File(suggestedFileName)
    }
    if (chooser.showSaveDialog(null) == JFileChooser.APPROVE_OPTION) {
        var file = chooser.selectedFile
        if (!file.name.endsWith(".md", ignoreCase = true)) file = File(file.parentFile, file.name + ".md")
        file.writeText(markdown)
    }
}

/** Native "Open" dialog for attaching a file's text content to a discussion. */
private fun pickFile(deliver: (fileName: String, content: String) -> Unit) {
    val chooser = JFileChooser().apply { dialogTitle = "Attach file" }
    if (chooser.showOpenDialog(null) == JFileChooser.APPROVE_OPTION) {
        val file = chooser.selectedFile
        runCatching { file.readText() }.onSuccess { deliver(file.name, it) }
    }
}

/** Thin shell: all screens live in `shared`; debates run on the Go engine. */
fun main(args: Array<String>) {
    System.setProperty("apple.awt.application.name", "Dialex")
    System.setProperty("apple.awt.application.appearance", "system")
    System.setProperty("apple.laf.useScreenMenuBar", "true")
    System.setProperty("com.apple.mrj.application.apple.menu.about.name", "Dialex")

    kolt.logutils.Log.init(true)
    installCrashLogger(File(System.getProperty("user.home"), ".dialex/crash.log"))
    if (args.contains("--persona-studio") || args.contains("--studio")) {
        com.dialex.desktop.personastudio.main(args)
    } else {
        mainApplication()
    }
}

private fun mainApplication() = application {
    val windowIcon = remember {
        runCatching {
            val stream = Thread.currentThread().contextClassLoader.getResourceAsStream("icon.png")
            stream?.use {
                androidx.compose.ui.res.loadImageBitmap(it)
            }?.let { androidx.compose.ui.graphics.painter.BitmapPainter(it) }
        }.getOrNull()
    }

    val windowState = rememberWindowState(
        width = 1140.dp,
        height = 760.dp,
        position = WindowPosition.Aligned(Alignment.Center)
    )

    Window(
        onCloseRequest = { EngineProcessManager.stop(); exitApplication() },
        title = "Dialex",
        icon = windowIcon,
        state = windowState
    ) {
        var showAboutDialog by remember { mutableStateOf(false) }

        LaunchedEffect(Unit) {
            window.title = "Dialex"
            window.minimumSize = Dimension(860, 580)
            runCatching {
                if (java.awt.Desktop.isDesktopSupported()) {
                    val desktop = java.awt.Desktop.getDesktop()
                    if (desktop.isSupported(java.awt.Desktop.Action.APP_ABOUT)) {
                        desktop.setAboutHandler {
                            showAboutDialog = true
                        }
                    }
                }
            }
            val os = System.getProperty("os.name", "").lowercase()
            if (os.contains("mac")) {
                window.rootPane.putClientProperty("apple.awt.fullWindowContent", true)
                window.rootPane.putClientProperty("apple.awt.transparentTitleBar", true)
                window.rootPane.putClientProperty("apple.awt.windowTitleVisible", false)
                runCatching {
                    val stream = Thread.currentThread().contextClassLoader.getResourceAsStream("icon.png")
                    val awtImage = stream?.use { javax.imageio.ImageIO.read(it) }
                    if (awtImage != null && java.awt.Taskbar.isTaskbarSupported()) {
                        val taskbar = java.awt.Taskbar.getTaskbar()
                        if (taskbar.isSupported(java.awt.Taskbar.Feature.ICON_IMAGE)) {
                            taskbar.iconImage = awtImage
                        }
                    }
                }
            } else {
                runCatching {
                    val stream = Thread.currentThread().contextClassLoader.getResourceAsStream("icon.png")
                    val awtImage = stream?.use { javax.imageio.ImageIO.read(it) }
                    if (awtImage != null) {
                        window.iconImage = awtImage
                    }
                }
            }
        }
        var themeMode by remember { mutableStateOf(ThemePreferenceStore.load()) }
        var engineClient by remember { mutableStateOf<com.dialex.engine.EngineClient?>(null) }
        var bootstrapError by remember { mutableStateOf<String?>(null) }
        var connecting by remember { mutableStateOf(false) }
        // null = no saved preference yet, ask; ThisDevice/Remote = decided (this run or a
        // past one) — ConnectionPreferenceStore is what makes that decision stick across
        // launches. A Remote preference still re-prompts for the password every launch: it's
        // never persisted (see ConnectionPreference's own doc comment).
        var preference by remember { mutableStateOf(ConnectionPreferenceStore.load()) }
        var cliStatus by mutableStateOf<Map<String, Boolean>?>(null)
        // Falls back to "all not found" rather than leaving cliStatus null forever if the
        // check itself throws for some environment-specific reason — the footer should
        // always show something, never silently disappear.
        suspend fun recheck() {
            cliStatus = runCatching { withContext(Dispatchers.IO) { checkCliAvailability() } }
                .getOrDefault(CliCatalog.associate { it.name to false })
        }

        suspend fun connect(makeClient: suspend () -> com.dialex.engine.EngineClient) {
            connecting = true
            bootstrapError = null
            runCatching {
                val client = withContext(Dispatchers.IO) { makeClient() }
                client
            }.onSuccess { engineClient = it }.onFailure { bootstrapError = it.message ?: it.toString() }
            connecting = false
            recheck()
        }

        // A saved "this device" preference boots straight in, same invisible flow as before
        // this screen existed — reuses an already-running engine (e.g. installed via
        // `roundtable service install`) or spawns + manages one itself, see
        // EngineProcessManager's doc comment. A saved remote preference skips straight to
        // the connect screen instead of the picker, prefilled, just needing the password.
        LaunchedEffect(preference) {
            if (preference is ConnectionPreference.ThisDevice) connect { EngineProcessManager.start() }
        }

        val toggleMaximize: () -> Unit = {
            val isMax = windowState.placement == androidx.compose.ui.window.WindowPlacement.Maximized ||
                    (window.extendedState and java.awt.Frame.MAXIMIZED_BOTH) != 0
            if (isMax) {
                windowState.placement = androidx.compose.ui.window.WindowPlacement.Floating
                window.extendedState = java.awt.Frame.NORMAL
            } else {
                windowState.placement = androidx.compose.ui.window.WindowPlacement.Maximized
            }
        }

        // One theme root for everything below
        AppTheme(themeMode) {
            com.dialex.App(
                engineClient = engineClient,
                supportsCli = true,
                supportsLocalEngine = true,
                themeMode = themeMode,
                onThemeModeChange = { mode ->
                    themeMode = mode
                    ThemePreferenceStore.save(mode)
                },
                onToggleMaximizeWindow = toggleMaximize,
                connectionLabel = when (val p = preference) {
                    is ConnectionPreference.ThisDevice -> "This device"
                    is ConnectionPreference.Remote -> "Remote: ${p.url}"
                    null -> ""
                },
                onSwitchConnection = {
                    EngineProcessManager.stop()
                    ConnectionPreferenceStore.clear()
                    preference = null
                    engineClient = null
                    bootstrapError = null
                },
                connectToLocalEngine = {
                    connect { EngineProcessManager.start() }
                },
                connectToRemote = { url, username, password ->
                    connect { com.dialex.engine.EngineClient(url).also { it.login(username, password) } }
                    if (bootstrapError == null) ConnectionPreferenceStore.save(ConnectionPreference.Remote(url, username))
                },
                recheckCli = {
                    val avail = withContext(Dispatchers.IO) { checkCliAvailability() }
                    val logins = withContext(Dispatchers.IO) { checkCliLoginStatus() }
                    CliCatalog.map { cli ->
                        val provider = when(cli.name) {
                            "Claude Code", "Claude" -> com.dialex.model.Provider.ANTHROPIC
                            "Codex (OpenAI)", "ChatGPT", "Codex" -> com.dialex.model.Provider.OPENAI
                            "Antigravity (Gemini)", "Gemini", "Antigravity" -> com.dialex.model.Provider.GEMINI
                            "Grok" -> com.dialex.model.Provider.GROK
                            "DeepSeek" -> com.dialex.model.Provider.DEEPSEEK
                            "Mistral" -> com.dialex.model.Provider.MISTRAL
                            else -> com.dialex.model.Provider.CUSTOM
                        }
                        val found = avail[cli.name] == true || avail[provider.name] == true
                        val loggedIn = logins[cli.name] == true || logins[provider.name] == true
                        com.dialex.presentation.settings.ProviderStatus(
                            provider = provider,
                            cliFound = found,
                            cliVersion = if (found) "installed" else null,
                            cliLoggedIn = found && loggedIn,
                            apiKeyConfigured = false
                        )
                    }
                },
                onExportMarkdown = { markdown, suggestedFileName ->
                    saveMarkdownAs(markdown, suggestedFileName)
                },
                extraSettingsTabLabel = "Servers",
                extraSettingsTabContent = { ServersTabContent() }
            )

            if (showAboutDialog) {
                com.dialex.ui.AboutDialog(
                    companyName = "Keltstone Systems",
                    onDismiss = { showAboutDialog = false }
                )
            }
        }
    }
}
