package com.example.hunteval.clean

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class SafeDeepLinkActivity : Activity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val data = intent.data ?: return
        // Mitigation: strict URI scheme and exact host validation
        if (data.scheme == "https" && data.host == "trusted.example.com") {
            webView.loadUrl(data.toString())
        }
    }
}
