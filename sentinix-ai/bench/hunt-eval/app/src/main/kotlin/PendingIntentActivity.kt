package com.example.hunteval

import android.app.Activity
import android.app.PendingIntent
import android.content.Intent
import android.os.Bundle

class PendingIntentActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Vulnerable: mutable PendingIntent created with implicit Intent allows hijacking action and extras
        val implicit = Intent()
        val pi = PendingIntent.getActivity(this, 100, implicit, PendingIntent.FLAG_MUTABLE)
        sendBroadcast(Intent("com.example.CALLBACK").putExtra("pi", pi))
    }
}
