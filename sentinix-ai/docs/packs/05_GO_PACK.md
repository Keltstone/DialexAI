# Sentinix AI: Go Security Pack (8 Rules)

**Pack Identifier:** `go`  
**Rule Count:** 8 Built-in Rules  
**Applicability Trigger:** Detection of Go source files (`*.go`) or `go.mod`.  
**Focus:** Memory safety, command execution, TLS misconfigurations, SQL injection, and debug endpoint exposure.

---

## Rule Catalogue

### 1. `go/sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing SQL queries using `fmt.Sprintf` or string concatenation.
* **Bad Pattern:** `db.Query(fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email))`
* **Good Pattern:** `db.Query("SELECT * FROM users WHERE email = $1", email)`

### 2. `go/insecure-tls-skip-verify`
* **Severity:** Critical | **CWE:** CWE-295 (Improper Certificate Validation)
* **Risk:** Disabling TLS certificate verification via `InsecureSkipVerify: true` makes connections vulnerable to Man-in-the-Middle (MITM) attacks.
* **Bad Pattern:** `&tls.Config{InsecureSkipVerify: true}`
* **Good Pattern:** `&tls.Config{MinVersion: tls.VersionTLS13}`

### 3. `go/weak-random-token`
* **Severity:** Medium | **CWE:** CWE-338 (Use of Cryptographically Weak Pseudo-Random Number Generator)
* **Risk:** Using `math/rand` instead of `crypto/rand` for security tokens, passwords, or session IDs.
* **Bad Pattern:** `token := mathrand.Intn(100000)`
* **Good Pattern:** `import "crypto/rand"; buf := make([]byte, 32); rand.Read(buf)`

### 4. `go/cors-allow-all-origins`
* **Severity:** Medium | **CWE:** CWE-942 (Overly Permissive Cross-Domain Whitelist)
* **Risk:** Setting `Access-Control-Allow-Origin: *` in HTTP middleware.
* **Bad Pattern:** `w.Header().Set("Access-Control-Allow-Origin", "*")`
* **Good Pattern:** Validate `r.Header.Get("Origin")` against a strict whitelist of trusted hosts.

### 5. `go/command-injection`
* **Severity:** Critical | **CWE:** CWE-78 (Improper Neutralization of Special Elements used in an OS Command)
* **Risk:** Passing unvalidated inputs into shell execution (`exec.Command("sh", "-c", userInput)` or `exec.Command("bash", "-c", ...)`).
* **Bad Pattern:** `exec.Command("sh", "-c", "ping "+host)`
* **Good Pattern:** `exec.Command("ping", "-c", "1", host)` without shell invocation.

### 6. `go/pprof-exposed-default-mux`
* **Severity:** High | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Importing `_ "net/http/pprof"` while using `http.ListenAndServe(":8080", nil)` exposes profiling endpoints on production ports.
* **Bad Pattern:** `import _ "net/http/pprof"` with `http.ListenAndServe(":8080", nil)`
* **Good Pattern:** Bind `pprof` handlers to a dedicated, internal localhost-only ServeMux.

### 7. `go/path-traversal-filepath-join`
* **Severity:** High | **CWE:** CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
* **Risk:** Using `filepath.Join(baseDir, userInput)` without verifying that the cleaned result resides inside `baseDir`.
* **Bad Pattern:** `os.ReadFile(filepath.Join(baseDir, userInput))`
* **Good Pattern:** `path := filepath.Clean(filepath.Join(baseDir, userInput)); if !strings.HasPrefix(path, filepath.Clean(baseDir)+string(filepath.Separator)) { return err }`

### 8. `go/unhandled-error-close`
* **Severity:** Low | **CWE:** CWE-703 (Improper Check or Handling of Exceptional Conditions)
* **Risk:** `defer res.Body.Close()` without checking error on write-flush operations.
* **Bad Pattern:** `defer res.Body.Close()`
* **Good Pattern:** Check returned error on close when flushing or writing buffers.
