package com.example.hunteval

import android.app.Activity
import android.os.Bundle
import java.io.File
import java.io.FileInputStream

class FileProviderActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val path = intent.getStringExtra("file_path") ?: return
        // Vulnerable: opens file without verifying canonical path stays inside filesDir
        val file = File(filesDir, path)
        val stream = FileInputStream(file)
        stream.close()
    }
}
