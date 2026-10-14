fun save(ctx: Context, db: SQLiteDatabase, name: String, pw: String) {
    ctx.openFileOutput("f", Context.MODE_WORLD_READABLE)
    ctx.getSharedPreferences("p", 0).edit().putString("password", pw).apply()
    MessageDigest.getInstance("MD5")
    db.rawQuery("select * from users where name = '$name'", null)
}
