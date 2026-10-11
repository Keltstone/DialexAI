package ai.artix.ide

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.execution.util.ExecUtil
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.ProgressManager
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.fileEditor.FileEditorManager
import com.google.gson.JsonObject
import com.google.gson.JsonParser

private fun notify(p: Project, type: NotificationType, msg: String) =
    NotificationGroupManager.getInstance().getNotificationGroup("Artix").createNotification("Artix", msg, type).notify(p)

/** Runs `artix <sub> --json [--provider/--model from env] <args>`; never autonomous. */
private fun runArtix(p: Project, sub: String, vararg args: String, onDone: (JsonObject?, String) -> Unit) {
    ProgressManager.getInstance().run(object : Task.Backgroundable(p, "Artix $sub", true) {
        override fun run(indicator: ProgressIndicator) {
            val env = System.getenv()
            val model = buildList {
                env["ARTIX_PROVIDER"]?.takeIf { it.isNotBlank() }?.let { add("--provider"); add(it) }
                env["ARTIX_MODEL"]?.takeIf { it.isNotBlank() }?.let { add("--model"); add(it) }
            }
            val cmd = GeneralCommandLine(env["ARTIX_BIN"] ?: "artix", sub, "--json").withParameters(model).withParameters(*args)
                .withWorkDirectory(p.basePath)
            val out = ExecUtil.execAndGetOutput(cmd, 30 * 60 * 1000)
            val json = runCatching { JsonParser.parseString(out.stdout.trim().lines().last()).asJsonObject }.getOrNull()
            onDone(json, out.stderr)
        }
    })
}

private fun JsonObject.str(k: String) = get(k)?.takeIf { !it.isJsonNull }?.asString

class PlanAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val p = e.project ?: return
        val story = Messages.showInputDialog(p, "User story", "Artix Plan", null) ?: return
        runArtix(p, "plan", story) { j, err ->
            val path = j?.str("specPath")
            if (path == null) return@runArtix notify(p, NotificationType.ERROR, "Plan failed: ${err.takeLast(400)}")
            notify(p, NotificationType.INFORMATION, "Spec: $path")
            LocalFileSystem.getInstance().refreshAndFindFileByPath(path)?.let { FileEditorManager.getInstance(p).openFile(it, true) }
        }
    }
}

class CodeAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val p = e.project ?: return
        runArtix(p, "code", "--print-test-commands") { printJson, err ->
            val ok = printJson?.get("ok")?.asBoolean == true
            val testCmds = printJson?.getAsJsonArray("testCommands")?.map { it.asString } ?: emptyList()
            val testCommandsHash = printJson?.str("testCommandsHash")
            if (!ok || testCmds.isEmpty() || testCommandsHash.isNullOrBlank()) {
                notify(p, NotificationType.ERROR, "Artix code: no test commands available to confirm (${printJson?.str("error") ?: err.takeLast(400)}). Run 'artix plan' first.")
                return@runArtix
            }
            val cmdListStr = testCmds.joinToString("\n") { "  • $it" }
            val msg = "Artix supervised mode will execute the following test commands:\n$cmdListStr\n\nHash: $testCommandsHash\n\nDo you want to proceed?"
            val confirmed = Messages.showOkCancelDialog(
                p,
                msg,
                "Confirm Test Commands",
                "Confirm & Run",
                "Cancel",
                Messages.getQuestionIcon()
            )
            if (confirmed != Messages.OK) return@runArtix
            runArtix(p, "code", "--autonomy", "supervised", "--confirm-tests", "--confirm-tests-hash", testCommandsHash) { j, err2 ->
                val status = j?.str("status")
                val isAwaiting = status == "awaiting_approval" || j?.get("awaitingApproval")?.asBoolean == true
                if (isAwaiting) {
                    notify(p, NotificationType.INFORMATION, "Candidate commit ${j?.str("commitHash") ?: ""} pushed to PR branch. Awaiting human approval on forge.")
                } else if (j?.get("success")?.asBoolean == true) {
                    val commitHash = j?.str("commitHash")
                    if (!commitHash.isNullOrBlank()) {
                        notify(p, NotificationType.INFORMATION, "Converged in ${j.get("roundsRun")} round(s). Committed: $commitHash")
                    } else {
                        notify(p, NotificationType.INFORMATION, "Converged in ${j.get("roundsRun")} round(s). Review the diff; nothing was committed.")
                    }
                } else {
                    notify(p, NotificationType.ERROR, "Code failed: ${j?.str("error") ?: err2.takeLast(400)}")
                }
            }
        }
    }
}

class ReviewAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val p = e.project ?: return
        runArtix(p, "review") { j, err ->
            when (j?.str("status")) {
                "approved" -> notify(p, NotificationType.INFORMATION, "Review passed: ${j.str("summary")}")
                "unreviewed" -> notify(p, NotificationType.WARNING, "Unreviewed: no model Critic ran (set ARTIX_PROVIDER/ARTIX_MODEL). NOT an approval.")
                "rejected" -> notify(p, NotificationType.ERROR, "Review rejected: ${j.str("summary")}\n${j.str("actionableFeedback") ?: ""}")
                else -> notify(p, NotificationType.ERROR, "Review failed: ${err.takeLast(400)}")
            }
        }
    }
}

class ShowDiffAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val p = e.project ?: return
        val panel = ArtixPanelRegistry.get(p)
        if (panel != null) {
            panel.showLatestDiff()
        } else {
            notify(p, NotificationType.INFORMATION, "Open Artix Tool Window to view live candidate diffs.")
        }
    }
}
