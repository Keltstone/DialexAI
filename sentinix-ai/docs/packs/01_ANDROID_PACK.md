# Sentinix AI: Android Security Pack (60 Rules)

**Pack Identifier:** `android`  
**Rule Count:** 60 Built-in Rules  
**Applicability Trigger:** Detection of `AndroidManifest.xml` or Gradle build files containing `com.android.application` or `com.android.library`.  
**Standards Alignment:** OWASP Mobile Application Security Verification Standard (MASVS v2.0) L1 & L2, Google Play Security Guidelines, Banking Grade App Hardening.

---

## 1. Categorical Breakdown

| Category | Rules Count | Description |
|---|---|---|
| **A. Manifest & Component Security (IPC)** | 15 rules | Exported activities/services, intent filters, app links, custom schemes, task hijacking. |
| **B. Data Storage & Privacy DLP** | 12 rules | World-readable files, SharedPreferences secrets, cloud backups, clipboard, Room encryption. |
| **C. Cryptography & Hardware Keystore** | 14 rules | Weak ciphers, static IVs, hardcoded keys, biometric invalidation, infinite key timeouts. |
| **D. Network & WebView Hardening** | 11 rules | Cleartext traffic, invalid trust managers, pinned certs, JS interfaces, file access. |
| **E. Runtime, Memory & System Integrity** | 8 rules | Screen recording protection, dynamic code loading, mock locations, unsafe native libs. |

---

## 2. Category A: Manifest & Component Security (IPC)

### 1. `exported-component`
* **Severity:** High | **CWE:** CWE-926 (Improper Export of Android Application Components)
* **Risk:** Exported activities, services, or broadcast receivers without explicit permission guards can be invoked by rogue applications installed on the same device.
* **Bad Pattern:** `<activity android:name=".PaymentActivity" android:exported="true">` (without permission)
* **Good Pattern:** `<activity android:name=".PaymentActivity" android:exported="false">` or guarded with `android:permission="com.app.permission.PAY"`.

### 2. `insecure-broadcast-receiver`
* **Severity:** Medium | **CWE:** CWE-925 (Improper Verification of Intent by Broadcast Receiver)
* **Risk:** Dynamic broadcast receivers registered without specifying a broadcast permission can receive spoofed broadcasts.
* **Bad Pattern:** `context.registerReceiver(receiver, filter)`
* **Good Pattern:** `ContextCompat.registerReceiver(context, receiver, filter, ContextCompat.RECEIVER_NOT_EXPORTED)`

### 3. `android/implicit-intent-broadcast`
* **Severity:** Medium | **CWE:** CWE-927 (Use of Implicit Intent for Sensitive Communication)
* **Risk:** Broadcasting an implicit Intent allows any eavesdropping app with a matching intent-filter to intercept sensitive payload data.
* **Bad Pattern:** `context.sendBroadcast(Intent("com.app.ACTION_UPDATE"))`
* **Good Pattern:** `val intent = Intent("com.app.ACTION_UPDATE").apply { setPackage(context.packageName) }; context.sendBroadcast(intent)`

### 4. `android/sticky-broadcast-deprecated`
* **Severity:** Medium | **CWE:** CWE-925 (Improper Verification of Intent by Broadcast Receiver)
* **Risk:** Sticky broadcasts stay cached by the OS and can be read or modified by any app without permissions.
* **Bad Pattern:** `context.sendStickyBroadcast(intent)`
* **Good Pattern:** Use non-sticky broadcasts, `SharedFlow`, or `LiveData`.

### 5. `android/intent-redirection`
* **Severity:** High | **CWE:** CWE-926 (Improper Export of Android Application Components)
* **Risk:** Launching an embedded Intent received from an external caller without sanitization allows attackers to access private components.
* **Bad Pattern:** `val target = intent.getParcelableExtra<Intent>("target"); startActivity(target)`
* **Good Pattern:** Validate `target.component` and ensure it belongs to an explicitly allowed internal whitelist.

### 6. `android/content-provider-grant-uri-all`
* **Severity:** High | **CWE:** CWE-732 (Incorrect Permission Assignment for Critical Resource)
* **Risk:** Setting `grantUriPermissions="true"` without restricting paths allows unauthorized applications to access all provider data.
* **Bad Pattern:** `<provider android:grantUriPermissions="true" android:exported="false"/>`
* **Good Pattern:** Use explicit child elements `<grant-uri-permission android:pathPrefix="/public/"/>`.

### 7. `android/custom-permission-normal`
* **Severity:** Medium | **CWE:** CWE-276 (Incorrect Default Permissions)
* **Risk:** Custom permissions declared with `protectionLevel="normal"` are granted automatically to any requesting application upon install.
* **Bad Pattern:** `<permission android:name="com.app.INTERNAL_IPC" android:protectionLevel="normal"/>`
* **Good Pattern:** `android:protectionLevel="signature"`

### 8. `android/task-hijacking-strandhogg`
* **Severity:** High | **CWE:** CWE-1021 (Improper Restriction of Rendered UI Layers or Frames)
* **Risk:** Activities with `launchMode="singleTask"` without an explicit empty `taskAffinity=""` are vulnerable to StrandHogg task hijacking.
* **Bad Pattern:** `<activity android:name=".MainActivity" android:launchMode="singleTask">`
* **Good Pattern:** `<activity android:name=".MainActivity" android:launchMode="singleTask" android:taskAffinity="">`

### 9. `android/app-link-missing-autoverify`
* **Severity:** High | **CWE:** CWE-601 (URL Redirection to Untrusted Site)
* **Risk:** HTTP/HTTPS intent filters lacking `android:autoVerify="true"` allow malicious apps to claim the domain and hijack OAuth redirection callbacks.
* **Bad Pattern:** `<intent-filter><action android:name="android.intent.action.VIEW"/><data android:scheme="https" android:host="app.com"/></intent-filter>`
* **Good Pattern:** `<intent-filter android:autoVerify="true">...`

### 10. `android/custom-scheme-oauth`
* **Severity:** High | **CWE:** CWE-926 (Improper Export of Android Application Components)
* **Risk:** Custom URI schemes (e.g. `myapp://oauth`) cannot be verified by Android OS and can be intercepted by any app claiming the same scheme.
* **Bad Pattern:** `<data android:scheme="myapp" android:host="oauth"/>`
* **Good Pattern:** Use verified HTTPS App Links with `autoVerify="true"`.

### 11. `android/content-provider-path-traversal`
* **Severity:** High | **CWE:** CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
* **Risk:** Implementing `ContentProvider.openFile()` without canonicalizing the path enables directory traversal via `../`.
* **Bad Pattern:** `File(baseDir, uri.lastPathSegment)`
* **Good Pattern:** `val file = File(baseDir, uri.lastPathSegment).canonicalFile; if (!file.path.startsWith(baseDir.canonicalPath)) throw SecurityException()`

### 12. `android/content-resolver-sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Passing raw string concatenations into `ContentResolver.query()` selection arguments enables SQL injection against internal providers.
* **Bad Pattern:** `contentResolver.query(uri, null, "user_id = '$userId'", null, null)`
* **Good Pattern:** `contentResolver.query(uri, null, "user_id = ?", arrayOf(userId), null)`

### 13. `android/shared-user-id`
* **Severity:** High | **CWE:** CWE-250 (Execution with Unnecessary Privileges)
* **Risk:** `android:sharedUserId` is deprecated and causes signature verification compromises and shared process vulnerabilities.
* **Bad Pattern:** `<manifest package="com.app" android:sharedUserId="com.shared.id">`
* **Good Pattern:** Remove `sharedUserId` and communicate via explicit IPC or ContentProviders.

### 14. `android/test-only-flag`
* **Severity:** Medium | **CWE:** CWE-489 (Active Debug Code)
* **Risk:** `android:testOnly="true"` left in production manifests exposes internal testing endpoints.
* **Bad Pattern:** `<application android:testOnly="true">`
* **Good Pattern:** Ensure release builds omit `android:testOnly`.

### 15. `android/unbounded-broadcast-permission`
* **Severity:** Medium | **CWE:** CWE-732 (Incorrect Permission Assignment for Critical Resource)
* **Risk:** Broadcast permissions defined without signature enforcement allow arbitrary apps to listen to private broadcast data.
* **Bad Pattern:** `<permission android:name="com.app.BROADCAST_DATA"/>`
* **Good Pattern:** `<permission android:name="com.app.BROADCAST_DATA" android:protectionLevel="signature"/>`

---

## 3. Category B: Data Storage & Privacy DLP

### 16. `world-accessible-file`
* **Severity:** Critical | **CWE:** CWE-276 (Incorrect Default Permissions)
* **Risk:** `MODE_WORLD_READABLE` or `MODE_WORLD_WRITEABLE` files can be read or modified by any application on the device.
* **Bad Pattern:** `openFileOutput("data.txt", Context.MODE_WORLD_READABLE)`
* **Good Pattern:** `openFileOutput("data.txt", Context.MODE_PRIVATE)`

### 17. `prefs-secret`
* **Severity:** Medium | **CWE:** CWE-312 (Cleartext Storage of Sensitive Information)
* **Risk:** Storing access tokens, passwords, or session keys in plain XML SharedPreferences.
* **Bad Pattern:** `sharedPreferences.edit().putString("auth_token", token).apply()`
* **Good Pattern:** Use `EncryptedSharedPreferences.create()` from AndroidX Security.

### 18. `allow-backup`
* **Severity:** Low | **CWE:** CWE-530 (Exposure of Structured Data through Data Forwarding)
* **Risk:** `android:allowBackup="true"` allows ADB backup commands to extract local application data to a connected computer.
* **Bad Pattern:** `<application android:allowBackup="true">`
* **Good Pattern:** `<application android:allowBackup="false">` or provide selective backup rules via `android:dataExtractionRules`.

### 19. `android/external-storage-write`
* **Severity:** High | **CWE:** CWE-276 (Incorrect Default Permissions)
* **Risk:** Writing sensitive application data to shared external storage (`Environment.getExternalStorageDirectory()`).
* **Bad Pattern:** `File(Environment.getExternalStorageDirectory(), "credentials.json")`
* **Good Pattern:** Use internal sandbox storage: `File(context.filesDir, "credentials.json")`.

### 20. `android/clipboard-data-leak`
* **Severity:** Low | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Copying sensitive tokens, credentials, or card numbers directly to system clipboard.
* **Bad Pattern:** `clipboardManager.setPrimaryClip(ClipData.newPlainText("secret", sensitiveToken))`
* **Good Pattern:** Set `ClipDescription.EXTRA_IS_SENSITIVE = true` on API 33+ or avoid putting credentials on clipboard.

### 21. `android/backup-rule-includes-credentials`
* **Severity:** High | **CWE:** CWE-530 (Exposure of Structured Data through Data Forwarding)
* **Risk:** Cloud backup rules (`backup_rules.xml`) that include authentication tokens or credential databases.
* **Bad Pattern:** `<include domain="sharedpref" path="auth_tokens.xml"/>`
* **Good Pattern:** `<exclude domain="sharedpref" path="auth_tokens.xml"/>`

### 22. `android/keyboard-personalized-learning-leak`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Sensitive input fields lacking `flagNoPersonalizedLearning` cause third-party keyboards to log input to cloud dictionaries.
* **Bad Pattern:** `<EditText android:id="@+id/pin_input" android:imeOptions="actionDone"/>`
* **Good Pattern:** `<EditText android:imeOptions="actionDone|flagNoPersonalizedLearning"/>`

### 23. `android/notification-lockscreen-leak`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Notifications containing sensitive transaction details set to `VISIBILITY_PUBLIC` display on locked devices.
* **Bad Pattern:** `builder.setVisibility(NotificationCompat.VISIBILITY_PUBLIC)`
* **Good Pattern:** `builder.setVisibility(NotificationCompat.VISIBILITY_PRIVATE)`

### 24. `android/unencrypted-room-database`
* **Severity:** Medium | **CWE:** CWE-312 (Cleartext Storage of Sensitive Information)
* **Risk:** Room database instantiated without SQLCipher database factory leaves SQLite data unencrypted on flash memory.
* **Bad Pattern:** `Room.databaseBuilder(context, AppDatabase::class.java, "app.db").build()`
* **Good Pattern:** `Room.databaseBuilder(...).openHelperFactory(SupportFactory(passphrase)).build()`

### 25. `android/insecure-webview-cache`
* **Severity:** Low | **CWE:** CWE-312 (Cleartext Storage of Sensitive Information)
* **Risk:** Leaving sensitive authenticated pages cached on disk in WebView storage across logout.
* **Bad Pattern:** Logging out without calling `webView.clearCache(true)`.
* **Good Pattern:** `webView.clearCache(true); webView.clearHistory(); CookieManager.getInstance().removeAllCookies(null)`

### 26. `android/file-provider-external-path`
* **Severity:** High | **CWE:** CWE-552 (Files or Directories Accessible to External Parties)
* **Risk:** FileProvider XML containing `<external-path>` grants external apps access to broad storage volumes.
* **Bad Pattern:** `<external-path name="external" path="."/>`
* **Good Pattern:** `<files-path name="internal_docs" path="docs/"/>`

### 27. `sql-string-template`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Executing SQLite queries assembled via Kotlin string templates (`$var`).
* **Bad Pattern:** `db.execSQL("DELETE FROM accounts WHERE id = '$accountId'")`
* **Good Pattern:** `db.execSQL("DELETE FROM accounts WHERE id = ?", arrayOf(accountId))`

---

## 4. Category C: Cryptography & Hardware Keystore

### 28. `weak-crypto`
* **Severity:** High | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Use of broken cryptographic algorithms such as DES, 3DES, RC4, or MD5.
* **Bad Pattern:** `Cipher.getInstance("DES/CBC/PKCS5Padding")`
* **Good Pattern:** `Cipher.getInstance("AES/GCM/NoPadding")`

### 29. `android/static-iv-cbc`
* **Severity:** High | **CWE:** CWE-329 (Generation of Predictable IV with CBC Mode)
* **Risk:** Hardcoding a static byte array or zeroed bytes as an Initialization Vector (IV) in CBC mode.
* **Bad Pattern:** `IvParameterSpec(byteArrayOf(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0))`
* **Good Pattern:** `val iv = ByteArray(16).also { SecureRandom().nextBytes(it) }; IvParameterSpec(iv)`

### 30. `android/insecure-random`
* **Severity:** Medium | **CWE:** CWE-338 (Use of Cryptographically Weak Pseudo-Random Number Generator)
* **Risk:** Using `java.util.Random` for cryptographic keys, OTPs, or password generation.
* **Bad Pattern:** `val token = java.util.Random().nextInt(1000000)`
* **Good Pattern:** `val rng = SecureRandom()`

### 31. `hardcoded-keystore-password`
* **Severity:** Critical | **CWE:** CWE-798 (Use of Hard-coded Credentials)
* **Risk:** Hardcoded Keystore passwords inside source code.
* **Bad Pattern:** `keyStore.load(stream, "SecretPassword123".toCharArray())`
* **Good Pattern:** Use AndroidKeyStore provider where private keys are hardware-isolated without software passwords.

### 32. `android/hardcoded-aes-key`
* **Severity:** Critical | **CWE:** CWE-798 (Use of Hard-coded Credentials)
* **Risk:** AES secret key embedded as a static string or byte array literal.
* **Bad Pattern:** `SecretKeySpec("MyStaticSecretKey123".toByteArray(), "AES")`
* **Good Pattern:** Generate keys dynamically inside the hardware Android KeyStore.

### 33. `android/rsa-no-padding`
* **Severity:** High | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** RSA cipher initialized with `NoPadding` (Textbook RSA), vulnerable to replay and mathematical recovery attacks.
* **Bad Pattern:** `Cipher.getInstance("RSA/ECB/NoPadding")`
* **Good Pattern:** `Cipher.getInstance("RSA/ECB/OAEPWithSHA-256AndMGF1Padding")`

### 34. `android/biometric-weak-crypto`
* **Severity:** High | **CWE:** CWE-287 (Improper Authentication)
* **Risk:** Prompting biometric authentication without passing a `BiometricPrompt.CryptoObject` for financial operations.
* **Bad Pattern:** `biometricPrompt.authenticate(promptInfo)`
* **Good Pattern:** `biometricPrompt.authenticate(promptInfo, BiometricPrompt.CryptoObject(cipher))`

### 35. `android/biometric-enrollment-invalidation`
* **Severity:** High | **CWE:** CWE-287 (Improper Authentication)
* **Risk:** Disabling biometric enrollment key invalidation allows a secondary enrolled fingerprint to decrypt user keys.
* **Bad Pattern:** `keyGenParameterSpecBuilder.setInvalidatedByBiometricEnrollment(false)`
* **Good Pattern:** `keyGenParameterSpecBuilder.setInvalidatedByBiometricEnrollment(true)`

### 36. `android/securerandom-fixed-seed`
* **Severity:** High | **CWE:** CWE-330 (Use of Insufficiently Random Values)
* **Risk:** Manually calling `SecureRandom.setSeed(...)` with a static seed defeats Linux `/dev/urandom` entropy pool.
* **Bad Pattern:** `SecureRandom().apply { setSeed(12345L) }`
* **Good Pattern:** `val rng = SecureRandom()` (let OS seed from hardware entropy).

### 37. `android/hardcoded-google-api-key`
* **Severity:** High | **CWE:** CWE-798 (Use of Hard-coded Credentials)
* **Risk:** Google API key starting with `AIzaSy` hardcoded in source.
* **Bad Pattern:** `val apiKey = "AIzaSyB1234567890abcdefghijklmnopqrstuv"`
* **Good Pattern:** Load from `BuildConfig` and apply Android package/SHA-1 fingerprint restrictions in Google Cloud Console.

### 38. `android/keystore-user-auth-timeout-infinite`
* **Severity:** High | **CWE:** CWE-305 (Authentication Bypass by Primary Weakness)
* **Risk:** Setting `setUserAuthenticationValidityDurationSeconds(-1)` or a huge number leaves keys unlocked permanently after initial auth.
* **Bad Pattern:** `builder.setUserAuthenticationValidityDurationSeconds(Integer.MAX_VALUE)`
* **Good Pattern:** `builder.setUserAuthenticationRequired(true); builder.setUserAuthenticationValidityDurationSeconds(30)`

### 39. `android/weak-kdf-iteration-count`
* **Severity:** High | **CWE:** CWE-326 (Inadequate Encryption Strength)
* **Risk:** PBKDF2 key derivation constructed with fewer than 100,000 iterations.
* **Bad Pattern:** `PBEKeySpec(password, salt, 1000, 256)`
* **Good Pattern:** `PBEKeySpec(password, salt, 120000, 256)`

### 40. `pending-intent-mutability`
* **Severity:** High | **CWE:** CWE-927 (Use of Implicit Intent for Sensitive Communication)
* **Risk:** PendingIntent created with `FLAG_MUTABLE` without `FLAG_IMMUTABLE` on Android 12+ (API 31+).
* **Bad Pattern:** `PendingIntent.getActivity(context, 0, intent, PendingIntent.FLAG_MUTABLE)`
* **Good Pattern:** `PendingIntent.getActivity(context, 0, intent, PendingIntent.FLAG_IMMUTABLE)`

---

## 5. Category D: Network & WebView Hardening

### 41. `cleartext-traffic`
* **Severity:** High | **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
* **Risk:** `android:usesCleartextTraffic="true"` allows the application to send HTTP traffic in plaintext.
* **Bad Pattern:** `<application android:usesCleartextTraffic="true">`
* **Good Pattern:** `<application android:usesCleartextTraffic="false">`

### 42. `trust-all-certs`
* **Severity:** Critical | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Custom `X509TrustManager` with empty `checkServerTrusted()` accepts any invalid, expired, or MITM certificate.
* **Bad Pattern:** `override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) {}`
* **Good Pattern:** Use default system trust manager or OkHttp `CertificatePinner`.

### 43. `webview-ssl-proceed`
* **Severity:** Critical | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Calling `handler.proceed()` inside `WebViewClient.onReceivedSslError()` bypasses SSL certificate errors.
* **Bad Pattern:** `override fun onReceivedSslError(view: WebView, handler: SslErrorHandler, error: SslError) { handler.proceed() }`
* **Good Pattern:** `handler.cancel()`

### 44. `webview-js-interface`
* **Severity:** High | **CWE:** CWE-749 (Exposed Dangerous Method or Function)
* **Risk:** Exposing Java objects to WebView JavaScript via `addJavascriptInterface()` without strict origin checks.
* **Bad Pattern:** `webView.addJavascriptInterface(Bridge(), "AndroidBridge")`
* **Good Pattern:** Ensure target origin is verified before executing bridge calls.

### 45. `webview-file-access`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Enabling `setAllowFileAccess(true)` or `setAllowUniversalAccessFromFileURLs(true)`.
* **Bad Pattern:** `webView.settings.allowFileAccess = true; webView.settings.allowUniversalAccessFromFileURLs = true`
* **Good Pattern:** Keep file access disabled. Use AndroidX `WebViewAssetLoader` for local assets.

### 46. `android/webview-save-passwords`
* **Severity:** Low | **CWE:** CWE-522 (Insufficiently Protected Credentials)
* **Risk:** Deprecated `setSavePassword(true)` in WebView.
* **Bad Pattern:** `webView.settings.savePassword = true`
* **Good Pattern:** `webView.settings.savePassword = false`

### 47. `android/custom-hostname-verifier-all`
* **Severity:** Critical | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Custom HostnameVerifier that returns `true` for all hostnames.
* **Bad Pattern:** `HostnameVerifier { _, _ -> true }`
* **Good Pattern:** Use standard OkHttp or Android `HttpsURLConnection.getDefaultHostnameVerifier()`.

### 48. `android/network-security-config-override-pins`
* **Severity:** High | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Setting `overridePins="true"` in Network Security Config disables pinning in production builds.
* **Bad Pattern:** `<pin-set overridePins="true">`
* **Good Pattern:** `<pin-set overridePins="false">`

### 49. `android/insecure-network-security-cleartext-domain`
* **Severity:** High | **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
* **Risk:** Declaring cleartext traffic permitted for subdomains in Network Security Config.
* **Bad Pattern:** `<domain-config cleartextTrafficPermitted="true"><domain includeSubdomains="true">api.com</domain></domain-config>`
* **Good Pattern:** `cleartextTrafficPermitted="false"`

### 50. `android/trustkit-wildcard-pins`
* **Severity:** Medium | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Setting `kTSKIncludeSubdomains: true` with a single pin risks app outage on subdomain certificate rotations.
* **Bad Pattern:** `TrustKit.init({"domain": {"kTSKIncludeSubdomains": true, "kTSKPublicKeyHashes": ["pin1"]}})`
* **Good Pattern:** Always specify backup pins: `["pin1", "backupPin"]`.

### 51. `android/ssl-error-toast-exposure`
* **Severity:** Low | **CWE:** CWE-209 (Generation of Error Message Containing Sensitive Information)
* **Risk:** Showing raw SSL error messages to users via Toast.
* **Bad Pattern:** `Toast.makeText(context, "SSL Error: " + error.toString(), Toast.LENGTH_LONG).show()`
* **Good Pattern:** Display a generic user-friendly connection failure message.

---

## 6. Category E: Runtime, Memory & System Integrity

### 52. `debuggable`
* **Severity:** Critical | **CWE:** CWE-489 (Active Debug Code)
* **Risk:** `android:debuggable="true"` in release AndroidManifest.xml enables JDWP debugger attachment and memory dumping.
* **Bad Pattern:** `<application android:debuggable="true">`
* **Good Pattern:** Ensure `debuggable` is omitted or set to `false` in release build types.

### 53. `tapjacking-protection`
* **Severity:** Medium | **CWE:** CWE-1021 (Improper Restriction of Rendered UI Layers or Frames)
* **Risk:** Disabling touch filtering allows malicious overlay apps to trick users into tapping buttons (Tapjacking).
* **Bad Pattern:** `view.setFilterTouchesWhenObscured(false)`
* **Good Pattern:** `view.setFilterTouchesWhenObscured(true)`

### 54. `android/screen-recording-sensitive-screen`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Screen capturing or recent-app snapshot leaks sensitive credentials or payment cards.
* **Bad Pattern:** Omitting `FLAG_SECURE` on login or payment activities.
* **Good Pattern:** `window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)`

### 55. `android/dynamic-code-loading`
* **Severity:** Critical | **CWE:** CWE-94 (Improper Control of Generation of Code)
* **Risk:** Loading executable DEX or JAR files at runtime using `DexClassLoader` or `PathClassLoader`.
* **Bad Pattern:** `DexClassLoader(dexPath, optimizedDir, null, classLoader)`
* **Good Pattern:** Bundle all code statically inside the signed APK / AAB.

### 56. `android/sensitive-log-leak`
* **Severity:** Medium | **CWE:** CWE-532 (Insertion of Sensitive Information into Log File)
* **Risk:** Writing passwords, credit cards, or access tokens to Logcat.
* **Bad Pattern:** `Log.d("AUTH", "User password=" + password)`
* **Good Pattern:** `Log.d("AUTH", "Authentication initiated")`

### 57. `android/unsafe-native-load`
* **Severity:** High | **CWE:** CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
* **Risk:** Loading `.so` native libraries from user-writable directories via `System.load()`.
* **Bad Pattern:** `System.load("/sdcard/libcustom.so")`
* **Good Pattern:** `System.loadLibrary("custom")` (loads from APK native library directory).

### 58. `android/coroutine-unconfined-dispatcher-auth`
* **Severity:** Low | **CWE:** CWE-662 (Improper Synchronization)
* **Risk:** Running sensitive authentication logic on `Dispatchers.Unconfined` risks unexpected thread hopping and race conditions.
* **Bad Pattern:** `withContext(Dispatchers.Unconfined) { authenticate() }`
* **Good Pattern:** `withContext(Dispatchers.IO) { authenticate() }`

### 59. `android/mock-location-allowed`
* **Severity:** Low | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Accepting GPS locations without checking `location.isFromMockProvider()` in geo-sensitive or fraud-detection apps.
* **Bad Pattern:** Using `location.latitude` without verifying mock status.
* **Good Pattern:** `if (Build.VERSION.SDK_INT >= 31 && location.isMock) throw SecurityException()`

### 60. `android/insecure-bluetooth-insecure-rfcomm`
* **Severity:** High | **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
* **Risk:** Creating insecure RFCOMM Bluetooth sockets without encryption or authentication.
* **Bad Pattern:** `device.createInsecureRfcommSocketToServiceRecord(uuid)`
* **Good Pattern:** `device.createRfcommSocketToServiceRecord(uuid)` (requires encrypted channel).
