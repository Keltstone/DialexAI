val base = "http://api.partner-example.net/v1"
fun login(token: String, settings: Settings) {
    println("token=$token")
    settings.putString("token", token)
}
