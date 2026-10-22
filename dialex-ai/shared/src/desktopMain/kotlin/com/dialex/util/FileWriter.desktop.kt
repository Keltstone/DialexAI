package com.dialex.util

import java.io.File

actual fun writeTextFile(folderPath: String, fileName: String, content: String, append: Boolean): Boolean {
    return try {
        val folder = File(folderPath)
        if (!folder.exists()) {
            folder.mkdirs()
        }
        val target = File(folder, fileName)
        if (append && target.exists()) {
            target.appendText("\n\n$content")
        } else {
            target.writeText(content)
        }
        true
    } catch (e: Exception) {
        kolt.logutils.printLog("Failed to write file to $folderPath/$fileName: ${e.message}", isError = true)
        false
    }
}
