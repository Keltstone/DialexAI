class DeepLinkActivity : Activity() {
    override fun onCreate(b: Bundle?) {
        val host = intent.data?.host ?: return
        if (host.endsWith("example.org")) webView.loadUrl(intent.data.toString())
    }
}
