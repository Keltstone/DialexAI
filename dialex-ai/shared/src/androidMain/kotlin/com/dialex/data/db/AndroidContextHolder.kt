package com.dialex.data.db

import android.content.ContentProvider
import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.net.Uri
import java.io.File

object AndroidContextHolder {
    @Volatile
    private var appContext: Context? = null

    fun init(context: Context) {
        appContext = context.applicationContext
    }

    fun getContext(): Context? = appContext

    fun getDatabasePath(dbName: String = "dialex.db"): String {
        val ctx = appContext
        return if (ctx != null) {
            val dbFile = ctx.getDatabasePath(dbName)
            dbFile.parentFile?.mkdirs()
            dbFile.absolutePath
        } else {
            val fallback = File("/data/data/com.keltstone.dialex/databases")
            fallback.mkdirs()
            File(fallback, dbName).absolutePath
        }
    }
}

class DialexInitProvider : ContentProvider() {
    override fun onCreate(): Boolean {
        context?.let { AndroidContextHolder.init(it) }
        return true
    }

    override fun query(uri: Uri, projection: Array<out String>?, selection: String?, selectionArgs: Array<out String>?, sortOrder: String?): Cursor? = null
    override fun getType(uri: Uri): String? = null
    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int = 0
}
