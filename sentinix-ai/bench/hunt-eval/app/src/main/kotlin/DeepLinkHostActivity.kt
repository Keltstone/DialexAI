package com.example.hunteval

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class DeepLinkHostActivity : Activity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val host = intent.data?.host ?: return
        // Vulnerable: host validation uses endsWith permitting attacker-controlled domains (e.g. evil-example.com)
        if (host.endsWith("example.com")) {
            webView.loadUrl(intent.data.toString())
        }
    }
}
