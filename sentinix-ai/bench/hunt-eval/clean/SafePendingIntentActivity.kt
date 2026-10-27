package com.example.hunteval.clean

import android.app.Activity
import android.app.PendingIntent
import android.content.Intent
import android.os.Bundle

class SafeService : Activity()

class SafePendingIntentActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Mitigation: FLAG_IMMUTABLE and explicit component prevent intent hijacking
        val explicit = Intent(this, SafeService::class.java)
        val pi = PendingIntent.getActivity(this, 101, explicit, PendingIntent.FLAG_IMMUTABLE)
        sendBroadcast(Intent("com.example.SECURE_CALLBACK").putExtra("pi", pi))
    }
}
