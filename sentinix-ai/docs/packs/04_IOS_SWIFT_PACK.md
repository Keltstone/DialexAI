# Sentinix AI: iOS & Swift Security Pack (8 Rules)

**Pack Identifier:** `ios-swift`  
**Rule Count:** 8 Built-in Rules  
**Applicability Trigger:** Detection of Swift files (`*.swift`), `Info.plist`, `Podfile`, or `Package.swift`.  
**Standards Alignment:** Apple Security Guidelines, OWASP MASVS for iOS.

---

## Rule Catalogue

### 1. `ios/ats-disabled`
* **Severity:** High | **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
* **Risk:** Disabling App Transport Security (ATS) via `NSAllowsArbitraryLoads = true` in `Info.plist` allows insecure HTTP communication.
* **Bad Pattern:** `<key>NSAllowsArbitraryLoads</key><true/>`
* **Good Pattern:** Keep ATS enabled. Use domain-specific exceptions only where strictly necessary.

### 2. `ios/insecure-keychain-accessibility`
* **Severity:** High | **CWE:** CWE-312 (Cleartext Storage of Sensitive Information)
* **Risk:** Storing Keychain items with `kSecAttrAccessibleAlways` or `kSecAttrAccessibleAlwaysThisDeviceOnly` permits item decryption even when the device is locked.
* **Bad Pattern:** `[kSecAttrAccessible as String: kSecAttrAccessibleAlways]`
* **Good Pattern:** Use `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` or `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`.

### 3. `ios/weak-crypto-md5-sha1`
* **Severity:** Medium | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Using legacy CommonCrypto functions `CC_MD5` or `CC_SHA1`.
* **Bad Pattern:** `CC_MD5(data, len, digest)`
* **Good Pattern:** Use CryptoKit: `SHA256.hash(data: data)`

### 4. `ios/sqlite-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing raw SQLite queries via Swift string interpolation inside `sqlite3_exec`.
* **Bad Pattern:** `sqlite3_exec(db, "SELECT * FROM users WHERE id = '\(userId)'", nil, nil, nil)`
* **Good Pattern:** Use `sqlite3_prepare_v2` and `sqlite3_bind_text`.

### 5. `ios/uiwebview-deprecated`
* **Severity:** Critical | **CWE:** CWE-477 (Use of Outdated Function)
* **Risk:** Using deprecated `UIWebView`, which lacks out-of-process sandboxing and modern security protections.
* **Bad Pattern:** `let webView = UIWebView()`
* **Good Pattern:** Use `WKWebView`.

### 6. `ios/pasteboard-leak`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Writing sensitive data to `UIPasteboard.general` without local-only restrictions allows other apps or Apple Universal Clipboard to sync the data to nearby devices.
* **Bad Pattern:** `UIPasteboard.general.string = authToken`
* **Good Pattern:** `UIPasteboard.general.setItems([[UIPasteboard.typeListString[0]: authToken]], options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(60)])`

### 7. `ios/biometric-fallback-bypass`
* **Severity:** High | **CWE:** CWE-287 (Improper Authentication)
* **Risk:** Falling back to device passcode (`deviceOwnerAuthentication`) instead of enforcing strictly biometric credentials (`deviceOwnerAuthenticationWithBiometrics`) on sensitive transactions.
* **Bad Pattern:** `context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason)`
* **Good Pattern:** `context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason)`

### 8. `ios/file-protection-none`
* **Severity:** High | **CWE:** CWE-311 (Missing Encryption of Sensitive Data)
* **Risk:** Writing files with `FileProtectionType.none` disables hardware file-level encryption at rest.
* **Bad Pattern:** `try data.write(to: url, options: .noFileProtection)`
* **Good Pattern:** Use `.completeFileProtection` or `.completeFileProtectionUnlessOpen`.
