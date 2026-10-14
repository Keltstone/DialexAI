fun Application.module(conn: Connection, name: String) {
    install(CORS) { anyHost() }
    http.csrf().disable()
    http.authorizeHttpRequests { it.anyRequest().permitAll() }
    conn.prepareStatement("select * from t where n = '$name'")
}
