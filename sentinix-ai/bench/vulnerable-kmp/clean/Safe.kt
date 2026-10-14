fun safe(ctx: Context, db: SQLiteDatabase, name: String, settings: Settings) {
    ctx.openFileOutput("f", Context.MODE_PRIVATE)
    ctx.getSharedPreferences("p", 0).edit().putString("theme", "dark").apply()
    MessageDigest.getInstance("SHA-256")
    db.rawQuery("select * from users where name = ?", arrayOf(name))
    web.loadUrl("https://example.com")
    val dev = "http://localhost:8080"
    handler.cancel()
    install(CORS) { allowHost("app.example.com") }
}
suspend fun login(username: String, password: String) {
    val host = base.removePrefix("https://").removePrefix("http://").substringBefore('/')
}
