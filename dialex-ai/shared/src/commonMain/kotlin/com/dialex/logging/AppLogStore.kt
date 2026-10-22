package com.dialex.logging

import com.dialex.util.formatMessageTimestamp
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

enum class AppLogLevel(val label: String) {
    INFO("INFO"),
    WARN("WARN"),
    ERROR("ERROR")
}

data class AppLogEntry(
    val id: Long,
    val timestampMs: Long,
    val level: AppLogLevel,
    val tag: String,
    val message: String,
    val errorDetails: String? = null
) {
    fun formatTimestamp(): String = formatMessageTimestamp(timestampMs)

    fun toFormattedLine(): String {
        val errStr = if (errorDetails != null) "\n  $errorDetails" else ""
        return "[${formatTimestamp()}] [${level.label}] [$tag] $message$errStr"
    }
}

/**
 * Thread-safe in-memory ring buffer recording runtime events, network calls,
 * CLI executions, and errors for the in-app System Logs & Diagnostics viewer.
 */
object AppLogStore {
    private const val MAX_LOGS = 500
    private var sequenceId = 0L

    private val _logs = MutableStateFlow<List<AppLogEntry>>(emptyList())
    val logs: StateFlow<List<AppLogEntry>> = _logs.asStateFlow()

    fun log(level: AppLogLevel, tag: String, message: String, throwable: Throwable? = null) {
        val entry = AppLogEntry(
            id = ++sequenceId,
            timestampMs = System.currentTimeMillis(),
            level = level,
            tag = tag,
            message = message,
            errorDetails = throwable?.stackTraceToString()
        )

        // Also print to console for terminal debugging
        val consoleMsg = "[Dialex] [${level.label}] [$tag] $message"
        if (level == AppLogLevel.ERROR) {
            kolt.logutils.printLog(consoleMsg, isError = true)
        } else {
            kolt.logutils.printLog(consoleMsg, isError = false)
        }

        // Update in-memory reactive list
        val current = _logs.value
        val updated = if (current.size >= MAX_LOGS) {
            current.drop(current.size - MAX_LOGS + 1) + entry
        } else {
            current + entry
        }
        _logs.value = updated
    }

    fun info(tag: String, message: String) = log(AppLogLevel.INFO, tag, message)
    fun warn(tag: String, message: String) = log(AppLogLevel.WARN, tag, message)
    fun error(tag: String, message: String, throwable: Throwable? = null) = log(AppLogLevel.ERROR, tag, message, throwable)

    fun clear() {
        _logs.value = emptyList()
    }

    fun exportFormatted(): String {
        val entries = _logs.value
        if (entries.isEmpty()) return "--- No System Logs Recorded ---"
        return entries.joinToString("\n") { it.toFormattedLine() }
    }
}
