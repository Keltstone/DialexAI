package com.example.hunteval

import android.app.Activity
import android.content.Intent
import android.os.Bundle

class ConfusedDeputyActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val target = intent.getParcelableExtra<Intent>("target_intent") ?: return
        // Vulnerable: forwards caller's intent to start unexported private service without caller verification
        startService(target)
    }
}
