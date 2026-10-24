# Sentinix AI: PHP Security Pack (8 Rules)

**Pack Identifier:** `php`  
**Rule Count:** 8 Built-in Rules  
**Applicability Trigger:** Detection of PHP source files (`*.php`) or `composer.json`.  
**Focus:** SQL injection, arbitrary command execution, object deserialization, SSRF, and file inclusions.

---

## Rule Catalogue

### 1. `php/sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing SQL queries by concatenating raw `$_GET` or `$_POST` variables into `mysqli_query` or PDO.
* **Bad Pattern:** `mysqli_query($conn, "SELECT * FROM users WHERE id = " . $_GET['id'])`
* **Good Pattern:** Use prepared statements: `$stmt = $pdo->prepare("SELECT * FROM users WHERE id = ?"); $stmt->execute([$id]);`

### 2. `php/command-injection`
* **Severity:** Critical | **CWE:** CWE-78 (Improper Neutralization of Special Elements used in an OS Command)
* **Risk:** Invoking system shell functions (`system()`, `exec()`, `passthru()`, `shell_exec()`, backticks) with unsanitized parameters.
* **Bad Pattern:** `system("ping -c 1 " . $_GET['host'])`
* **Good Pattern:** Use `escapeshellarg()` or use native PHP networking functions instead of shell calls.

### 3. `php/insecure-unserialize`
* **Severity:** Critical | **CWE:** CWE-502 (Deserialization of Untrusted Data)
* **Risk:** Passing untrusted user data into `unserialize()` enables PHP Object Injection and Remote Code Execution.
* **Bad Pattern:** `$data = unserialize($_COOKIE['session'])`
* **Good Pattern:** Use `json_decode()` or specify allowed classes: `unserialize($data, ["allowed_classes" => false])`.

### 4. `php/xss-echo-unencoded`
* **Severity:** Medium | **CWE:** CWE-79 (Improper Neutralization of Input During Web Page Generation - XSS)
* **Risk:** Echoing raw request variables directly into HTML without output encoding.
* **Bad Pattern:** `echo "<div>Welcome, " . $_GET['name'] . "</div>"`
* **Good Pattern:** `echo "<div>Welcome, " . htmlspecialchars($_GET['name'], ENT_QUOTES, 'UTF-8') . "</div>"`

### 5. `php/remote-file-inclusion`
* **Severity:** Critical | **CWE:** CWE-98 (Improper Control of Generation of Code - Remote File Inclusion)
* **Risk:** Using user input directly inside `include`, `require`, `include_once`, or `require_once`.
* **Bad Pattern:** `include($_GET['page'] . ".php")`
* **Good Pattern:** Map input parameters to a strict whitelist array of allowed file paths.

### 6. `php/ssrf-curl`
* **Severity:** High | **CWE:** CWE-918 (Server-Side Request Forgery - SSRF)
* **Risk:** Setting `CURLOPT_URL` directly from unvalidated user input, permitting internal network probing.
* **Bad Pattern:** `curl_setopt($ch, CURLOPT_URL, $_POST['webhook_url'])`
* **Good Pattern:** Validate that the target host resolves to a public IP and does not match RFC 1918 / localhost addresses.

### 7. `php/weak-random-rand`
* **Severity:** Medium | **CWE:** CWE-338 (Use of Cryptographically Weak Pseudo-Random Number Generator)
* **Risk:** Using `rand()` or `mt_rand()` for security tokens, passwords, or salts.
* **Bad Pattern:** `$token = bin2hex(random_bytes(16)); // good vs $token = md5(rand())`
* **Good Pattern:** Use `random_bytes()` or `random_int()`.

### 8. `php/insecure-cookie-session`
* **Severity:** Medium | **CWE:** CWE-614 (Sensitive Cookie in HTTPS Session Without 'Secure' Attribute)
* **Risk:** Initializing session cookies with `secure = false` or `httponly = false`.
* **Bad Pattern:** `session_set_cookie_params(3600, "/", null, false, false)`
* **Good Pattern:** `session_set_cookie_params(['lifetime' => 3600, 'path' => '/', 'domain' => '', 'secure' => true, 'httponly' => true, 'samesite' => 'Strict'])`
