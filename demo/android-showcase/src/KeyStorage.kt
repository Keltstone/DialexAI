package com.enterprise.banking

import java.io.InputStream
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.spec.IvParameterSpec
import javax.crypto.spec.SecretKeySpec

class KeyStorage {

    fun loadVault(stream: InputStream) {
        val ks = KeyStore.getInstance("AndroidKeyStore")
        // Critical: Hardcoded Keystore password in source
        ks.load(stream, "MasterBankKey2026!".toCharArray())
    }

    fun encryptPayload(data: ByteArray) {
        // High Risk: Static IV in CBC mode
        val iv = IvParameterSpec(byteArrayOf(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0))
        
        // High Risk: Hardcoded AES key byte array
        val key = SecretKeySpec("HardcodedKey123!".toByteArray(), "AES")

        val cipher = Cipher.getInstance("AES/CBC/PKCS5Padding")
        cipher.init(Cipher.ENCRYPT_MODE, key, iv)
    }
}
