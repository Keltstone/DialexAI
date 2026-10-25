package com.example.hunteval.clean

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class SafeWebViewActivity : Activity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webView = WebView(this)
        // Mitigation: JavascriptInterface disabled, strict local content only
        webView.settings.javaScriptEnabled = false
        webView.loadUrl("file:///android_asset/about.html")
    }
}
