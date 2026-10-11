package ai.artix.ide

import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.intellij.diff.DiffContentFactory
import com.intellij.diff.DiffManager
import com.intellij.diff.requests.SimpleDiffRequest
import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.execution.process.OSProcessHandler
import com.intellij.execution.process.ProcessAdapter
import com.intellij.execution.process.ProcessEvent
import com.intellij.execution.process.ProcessOutputTypes
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.util.Key
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBPanel
import com.intellij.ui.components.JBScrollPane
import com.intellij.ui.components.JBTextArea
import com.intellij.ui.content.ContentFactory
import java.awt.BorderLayout
import java.awt.FlowLayout
import java.awt.Font
import javax.swing.BorderFactory
import javax.swing.JButton
import javax.swing.JPanel
import javax.swing.SwingUtilities

class ArtixToolWindowFactory : ToolWindowFactory {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val panel = ArtixPanel(project)
        val content = ContentFactory.getInstance().createContent(panel, "", false)
        toolWindow.contentManager.addContent(content)
        ArtixPanelRegistry.register(project, panel)
    }
}

object ArtixPanelRegistry {
    private val panels = mutableMapOf<Project, ArtixPanel>()
    fun register(p: Project, panel: ArtixPanel) { panels[p] = panel }
    fun get(p: Project): ArtixPanel? = panels[p]
}

class ArtixPanel(private val project: Project) : JBPanel<ArtixPanel>(BorderLayout()) {
    private val statusLabel = JBLabel("Status: Idle").apply {
        font = font.deriveFont(Font.BOLD, 13f)
        border = BorderFactory.createEmptyBorder(6, 8, 6, 8)
    }

    private val logArea = JBTextArea().apply {
        isEditable = false
        font = Font(Font.MONOSPACED, Font.PLAIN, 12)
        lineWrap = true
        wrapStyleWord = true
    }

    private var latestDiff: String? = null

    init {
        val topPanel = JPanel(BorderLayout()).apply {
            add(statusLabel, BorderLayout.WEST)
            val buttonsPanel = JPanel(FlowLayout(FlowLayout.RIGHT)).apply {
                val planBtn = JButton("Plan").apply { addActionListener { triggerPlan() } }
                val codeBtn = JButton("Code").apply { addActionListener { triggerCode() } }
                val reviewBtn = JButton("Review").apply { addActionListener { triggerReview() } }
                val diffBtn = JButton("View Diff").apply { addActionListener { showLatestDiff() } }
                val clearBtn = JButton("Clear").apply { addActionListener { clearLog() } }

                add(planBtn)
                add(codeBtn)
                add(reviewBtn)
                add(diffBtn)
                add(clearBtn)
            }
            add(buttonsPanel, BorderLayout.EAST)
        }

        add(topPanel, BorderLayout.NORTH)
        add(JBScrollPane(logArea), BorderLayout.CENTER)
    }

    fun setStatus(text: String) {
        SwingUtilities.invokeLater { statusLabel.text = "Status: $text" }
    }

    fun appendLog(line: String) {
        SwingUtilities.invokeLater {
            logArea.append(line + "\n")
            logArea.caretPosition = logArea.document.length
        }
    }

    fun clearLog() {
        SwingUtilities.invokeLater {
            logArea.text = ""
            statusLabel.text = "Status: Idle"
            latestDiff = null
        }
    }

    fun setLatestDiff(diff: String) {
        latestDiff = diff
    }

    fun showLatestDiff() {
        val diff = latestDiff
        if (diff.isNullOrBlank()) {
            NotificationGroupManager.getInstance().getNotificationGroup("Artix")
                .createNotification("Artix", "No recent diff available to view.", NotificationType.INFORMATION)
                .notify(project)
            return
        }

        val factory = DiffContentFactory.getInstance()
        val emptyContent = factory.create(project, "")
        val diffContent = factory.create(project, diff)
        val req = SimpleDiffRequest("Artix Candidate Changes", emptyContent, diffContent, "Base (HEAD)", "Candidate Patch")

        ApplicationManager.getApplication().invokeLater {
            DiffManager.getInstance().showDiff(project, req)
        }
    }

    private fun triggerPlan() {
        val story = Messages.showInputDialog(project, "User story prompt:", "Artix Plan Story", null) ?: return
        setStatus("Planning...")
        appendLog("=== Starting Artix Story Planning ===")
        appendLog("Prompt: $story\n")

        runStreamingArtix(project, "plan", story) { success, lastJson ->
            if (success) {
                val path = lastJson?.get("specPath")?.asString
                setStatus("Plan Generated")
                appendLog("\nPlan completed successfully: $path")
                if (path != null) {
                    LocalFileSystem.getInstance().refreshAndFindFileByPath(path)?.let {
                        ApplicationManager.getApplication().invokeLater {
                            FileEditorManager.getInstance(project).openFile(it, true)
                        }
                    }
                }
            } else {
                setStatus("Plan Failed")
            }
        }
    }

    private fun triggerCode() {
        // Step 1: Query planned test commands
        setStatus("Verifying Test Commands...")
        appendLog("=== Artix Supervised Code Loop ===")
        
        runArtixSync(project, "code", "--print-test-commands") { printJson, err ->
            val ok = printJson?.get("ok")?.asBoolean == true
            val testCmds = printJson?.getAsJsonArray("testCommands")?.map { it.asString } ?: emptyList()
            val testCommandsHash = printJson?.get("testCommandsHash")?.asString
            if (!ok || testCmds.isEmpty() || testCommandsHash.isNullOrBlank()) {
                val errMsg = "No test commands available (${printJson?.get("error")?.asString ?: err.takeLast(400)}). Run 'plan' first."
                setStatus("Error")
                appendLog("Error: $errMsg")
                NotificationGroupManager.getInstance().getNotificationGroup("Artix")
                    .createNotification("Artix", errMsg, NotificationType.ERROR).notify(project)
                return@runArtixSync
            }

            val cmdListStr = testCmds.joinToString("\n") { "  • $it" }
            val msg = "Artix supervised mode will execute the following test commands:\n$cmdListStr\n\nHash: $testCommandsHash\n\nConfirm test execution?"
            val confirmed = Messages.showOkCancelDialog(
                project, msg, "Confirm Test Commands", "Confirm & Run", "Cancel", Messages.getQuestionIcon()
            )
            if (confirmed != Messages.OK) {
                setStatus("Cancelled")
                appendLog("Execution cancelled by developer.")
                return@runArtixSync
            }

            setStatus("Converging...")
            appendLog("Test commands confirmed ($testCommandsHash). Starting convergence loop...\n")

            runStreamingArtix(project, "code", "--autonomy", "supervised", "--confirm-tests", "--confirm-tests-hash", testCommandsHash) { success, lastJson ->
                val status = lastJson?.get("status")?.asString
                val isAwaiting = status == "awaiting_approval" || lastJson?.get("awaitingApproval")?.asBoolean == true
                if (isAwaiting) {
                    setStatus("Awaiting Approval")
                    appendLog("\nCandidate commit staged/pushed. Awaiting developer approval.")
                } else if (success) {
                    setStatus("Converged")
                    val rounds = lastJson?.get("roundsRun")?.asInt ?: 1
                    val hash = lastJson?.get("commitHash")?.asString
                    appendLog("\nConvergence achieved in $rounds round(s). Commit: ${hash ?: "staged"}")
                } else {
                    setStatus("Convergence Failed")
                    appendLog("\nCode convergence failed: ${lastJson?.get("error")?.asString ?: "unknown"}")
                }
            }
        }
    }

    private fun triggerReview() {
        setStatus("Reviewing...")
        appendLog("=== Starting Adversarial Review ===")

        runStreamingArtix(project, "review") { _, lastJson ->
            val status = lastJson?.get("status")?.asString
            val approved = lastJson?.get("approved")?.asBoolean == true
            val summary = lastJson?.get("summary")?.asString ?: ""

            when (status) {
                "approved" -> {
                    setStatus("Review Passed")
                    appendLog("\nReview PASSED: $summary")
                }
                "unreviewed" -> {
                    setStatus("Unreviewed")
                    appendLog("\nReview UNREVIEWED: $summary (set ARTIX_PROVIDER/ARTIX_MODEL)")
                }
                "rejected" -> {
                    setStatus("Review Rejected")
                    appendLog("\nReview REJECTED: $summary")
                }
                else -> {
                    setStatus("Review Error")
                    appendLog("\nReview error: ${lastJson?.get("error")?.asString ?: summary}")
                }
            }
        }
    }

    private fun runStreamingArtix(
        p: Project,
        sub: String,
        vararg args: String,
        onFinished: (Boolean, JsonObject?) -> Unit
    ) {
        val env = System.getenv()
        val model = buildList {
            env["ARTIX_PROVIDER"]?.takeIf { it.isNotBlank() }?.let { add("--provider"); add(it) }
            env["ARTIX_MODEL"]?.takeIf { it.isNotBlank() }?.let { add("--model"); add(it) }
        }

        val cmd = GeneralCommandLine(env["ARTIX_BIN"] ?: "artix", sub, "--stream")
            .withParameters(model)
            .withParameters(*args)
            .withWorkDirectory(p.basePath)

        var lastParsedJson: JsonObject? = null

        try {
            val handler = OSProcessHandler(cmd)
            handler.addProcessListener(object : ProcessAdapter() {
                override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                    val text = event.text.trim()
                    if (text.isEmpty()) return

                    if (text.startsWith("{") && text.endsWith("}")) {
                        val json = runCatching { JsonParser.parseString(text).asJsonObject }.getOrNull()
                        if (json != null) {
                            lastParsedJson = json
                            val phase = json.get("phase")?.asString ?: ""
                            val msg = json.get("message")?.asString ?: ""
                            val round = json.get("round")?.asInt
                            val status = json.get("status")?.asString

                            val prefix = if (round != null && round > 0) "[$phase R$round]" else if (phase.isNotEmpty()) "[$phase]" else ""
                            if (prefix.isNotEmpty() || msg.isNotEmpty()) {
                                appendLog("$prefix $msg".trim())
                            }
                            if (phase.isNotEmpty()) {
                                setStatus(phase + if (round != null) " (R$round)" else "")
                            }
                            return
                        }
                    }

                    if (outputType == ProcessOutputTypes.STDERR) {
                        appendLog("[stderr] $text")
                    } else {
                        appendLog(text)
                    }
                }

                override fun processTerminated(event: ProcessEvent) {
                    val success = event.exitCode == 0
                    onFinished(success, lastParsedJson)
                }
            })
            handler.startNotify()
        } catch (e: Exception) {
            setStatus("Error")
            appendLog("Execution error: ${e.message}")
            onFinished(false, null)
        }
    }

    private fun runArtixSync(
        p: Project,
        sub: String,
        vararg args: String,
        onDone: (JsonObject?, String) -> Unit
    ) {
        ApplicationManager.getApplication().executeOnPooledThread {
            val env = System.getenv()
            val model = buildList {
                env["ARTIX_PROVIDER"]?.takeIf { it.isNotBlank() }?.let { add("--provider"); add(it) }
                env["ARTIX_MODEL"]?.takeIf { it.isNotBlank() }?.let { add("--model"); add(it) }
            }
            val cmd = GeneralCommandLine(env["ARTIX_BIN"] ?: "artix", sub, "--json")
                .withParameters(model)
                .withParameters(*args)
                .withWorkDirectory(p.basePath)

            val out = com.intellij.execution.util.ExecUtil.execAndGetOutput(cmd, 30 * 1000)
            val json = runCatching { JsonParser.parseString(out.stdout.trim().lines().last()).asJsonObject }.getOrNull()
            SwingUtilities.invokeLater {
                onDone(json, out.stderr)
            }
        }
    }
}
