package com.enterprise.banking

import android.app.Activity
import android.os.Bundle
import android.util.Log
import androidx.biometric.BiometricPrompt

class AuthActivity : Activity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Missing FLAG_SECURE on sensitive financial screen
        setContentView(0)
    }

    fun authenticateUser(prompt: BiometricPrompt, promptInfo: BiometricPrompt.PromptInfo) {
        // High Risk: Biometric prompt without CryptoObject for financial authorization
        prompt.authenticate(promptInfo)
    }

    fun handleLogin(userPin: String) {
        // Sensitive log leak: Writing user PIN to Logcat
        Log.d("AUTH_DEBUG", "User entered PIN: " + userPin)
    }
}
