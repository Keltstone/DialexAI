package com.example.hunteval.clean

import android.app.Activity
import android.os.Bundle
import java.io.File
import java.io.FileInputStream

class SafeFileProviderActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val path = intent.getStringExtra("file_path") ?: return
        val target = File(filesDir, path)
        // Mitigation: verifies canonical path is inside expected root directory
        if (!target.canonicalPath.startsWith(filesDir.canonicalPath)) {
            finish()
            return
        }
        val stream = FileInputStream(target)
        stream.close()
    }
}
