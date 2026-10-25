package com.example.hunteval

import android.app.Activity
import android.os.Bundle
import android.webkit.JavascriptInterface
import android.webkit.WebView

class JSInterface {
    @JavascriptInterface
    fun executePrivileged(cmd: String): String {
        return "executed"
    }
}

class WebViewBridgeActivity : Activity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        // Vulnerable: JavascriptInterface exposed on webview loading arbitrary external URLs
        webView.addJavascriptInterface(JSInterface(), "NativeBridge")
        val url = intent.getStringExtra("url") ?: "http://untrusted.org"
        webView.loadUrl(url)
    }
}
