package com.example.hunteval.clean

import android.app.Activity
import android.os.Bundle
import android.widget.Button

class SafeTapjackingActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val btn = Button(this)
        // Mitigation: filterTouchesWhenObscured prevents tapjacking overlays
        btn.filterTouchesWhenObscured = true
    }
}
