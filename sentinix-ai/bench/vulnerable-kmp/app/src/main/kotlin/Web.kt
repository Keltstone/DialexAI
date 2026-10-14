fun setup(web: WebView, settings: WebSettings) {
    web.addJavascriptInterface(Bridge(), "bridge")
    settings.allowUniversalAccessFromFileURLs = true
}
class Client : WebViewClient() {
    override fun onReceivedSslError(v: WebView, handler: SslErrorHandler, e: SslError) { handler.proceed() }
}
