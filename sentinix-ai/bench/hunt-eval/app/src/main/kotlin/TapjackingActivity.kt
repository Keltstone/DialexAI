package com.example.hunteval

import android.app.Activity
import android.os.Bundle
import android.widget.Button

class TapjackingActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val btn = Button(this)
        // Vulnerable: exported sensitive action screen lacks filterTouchesWhenObscured
        btn.setOnClickListener {
            // Transfer funds or grant sensitive permissions
        }
    }
}
