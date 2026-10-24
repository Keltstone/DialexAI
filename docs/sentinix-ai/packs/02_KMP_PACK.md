# Sentinix AI: Kotlin Multiplatform (KMP) Security Pack (8 Rules)

**Pack Identifier:** `kmp`  
**Rule Count:** 8 Built-in Rules  
**Applicability Trigger:** Detection of `commonMain` directory, or Gradle build files applying the `org.jetbrains.kotlin.multiplatform` plugin.  
**Focus:** Shared multiplatform architecture pitfalls, non-volatile state mutations, cleartext transmission across platform boundaries, and multiplatform settings vulnerabilities.

---

## Rule Catalogue

### 1. `common-cleartext-url`
* **Severity:** Medium | **CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
* **Risk:** Hardcoding unencrypted `http://` URLs in commonMain code exposes network calls across Android, iOS, Desktop, and Web.
* **Bad Pattern:** `val endpoint = "http://api.service.internal/v1/auth"`
* **Good Pattern:** `val endpoint = "https://api.service.internal/v1/auth"`

### 2. `common-log-secret`
* **Severity:** Medium | **CWE:** CWE-532 (Insertion of Sensitive Information into Log File)
* **Risk:** Printing passwords, auth tokens, or private secrets via shared logging libraries (Napier, Kermit, println).
* **Bad Pattern:** `Napier.d("User authenticated with token: $token")`
* **Good Pattern:** `Napier.d("User authentication succeeded")`

### 3. `settings-plain-secret`
* **Severity:** Medium | **CWE:** CWE-312 (Cleartext Storage of Sensitive Information)
* **Risk:** Storing authentication tokens or secrets in standard `multiplatform-settings` without hardware keychain/keystore encryption.
* **Bad Pattern:** `settings.putString("auth_token", token)`
* **Good Pattern:** Use an encrypted settings wrapper backed by Android EncryptedSharedPreferences and iOS Keychain (`KeychainSettings`).

### 4. `ktor-trust-all-certs`
* **Severity:** Critical | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Ktor HTTP Client configured with custom SSL engines that trust all certificates.
* **Bad Pattern:** `trustManager = TrustAllCerts` or disabling certificate validation in engine configuration.
* **Good Pattern:** Use standard system certificate trust stores.

### 5. `mutable-global-state`
* **Severity:** Medium | **CWE:** CWE-662 (Improper Synchronization)
* **Risk:** Declaring `var` properties inside companion objects or top-level objects in common code causes memory races and thread safety violations across platforms (especially Kotlin/Native).
* **Bad Pattern:** `companion object { var currentUser: User? = null }`
* **Good Pattern:** Use `AtomicReference` or `StateFlow`: `private val _currentUser = MutableStateFlow<User?>(null)`

### 6. `weak-hash-common`
* **Severity:** High | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Using MD5 or SHA-1 hash digests in multiplatform common logic.
* **Bad Pattern:** `Digest.md5(data)` or `MessageDigest.getInstance("MD5")`
* **Good Pattern:** Use SHA-256 or SHA-512.

### 7. `hardcoded-api-key`
* **Severity:** High | **CWE:** CWE-798 (Use of Hard-coded Credentials)
* **Risk:** Hardcoded API keys or client secrets stored directly in Kotlin source files.
* **Bad Pattern:** `val apiKey = "pk_live_51Abcdefghijklmnopqrstuv"`
* **Good Pattern:** Inject keys via `BuildKonfig` or platform expect/actual configuration loaders from environment variables.

### 8. `insecure-random-token`
* **Severity:** Medium | **CWE:** CWE-338 (Use of Cryptographically Weak Pseudo-Random Number Generator)
* **Risk:** `kotlin.random.Random` is a pseudo-random generator, not cryptographically secure, and must never be used for tokens, nonces, or salts.
* **Bad Pattern:** `val token = kotlin.random.Random.nextBytes(32)`
* **Good Pattern:** Use platform-specific cryptographic random generators (`expect`/`actual` backed by `SecureRandom` on JVM/Android and `SecRandomCopyBytes` on Apple).
