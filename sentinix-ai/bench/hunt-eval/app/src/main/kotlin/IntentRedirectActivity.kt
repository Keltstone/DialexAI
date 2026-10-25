package com.example.hunteval

import android.app.Activity
import android.content.Intent
import android.os.Bundle

class IntentRedirectActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val forward = intent.getParcelableExtra<Intent>("forward_intent") ?: return
        // Vulnerable: launches nested intent directly from exported activity without sanitization
        startActivity(forward)
    }
}
