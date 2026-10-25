class MapActivity : Activity() {
    override fun onCreate(b: Bundle?) {
        val url = intent.getStringExtra("tile_url")
        prefs.edit().putString("tile_url_template", url).apply()
    }
}
