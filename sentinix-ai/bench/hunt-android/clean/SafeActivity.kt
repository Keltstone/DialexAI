class SafeActivity : Activity() {
    override fun onCreate(b: Bundle?) {
        val host = intent.data?.host ?: return
        if (host == "example.org" || host.endsWith(".example.org")) webView.loadUrl(intent.data.toString())
    }
}
