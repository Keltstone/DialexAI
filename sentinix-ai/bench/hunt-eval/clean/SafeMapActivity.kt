package com.example.hunteval.clean

import android.app.Activity
import android.os.Bundle

class SafeMapActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Mitigation: strict caller identity check
        val caller = callingActivity?.packageName
        if (caller == null || caller != packageName) {
            finish()
            return
        }
    }
}
