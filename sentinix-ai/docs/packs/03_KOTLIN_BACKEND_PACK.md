# Sentinix AI: Kotlin Backend Security Pack (10 Rules)

**Pack Identifier:** `kotlin-backend`  
**Rule Count:** 10 Built-in Rules  
**Applicability Trigger:** Detection of Kotlin server-side frameworks (Spring Boot, Ktor Server, Quarkus, Micronaut).  
**Focus:** Cloud microservice hardening, API authorization bypasses, open CORS, CSRF, insecure XML parsers, and JWT configuration.

---

## Rule Catalogue

### 1. `spring-csrf-disabled`
* **Severity:** Medium | **CWE:** CWE-352 (Cross-Site Request Forgery - CSRF)
* **Risk:** Explicitly disabling CSRF protection in Spring Security (`http.csrf { it.disable() }`).
* **Bad Pattern:** `http.csrf().disable()` or `csrf { disable() }`
* **Good Pattern:** Keep CSRF enabled, or use cookie-to-header token repository for SPA clients: `csrf { csrfTokenRepository(CookieCsrfTokenRepository.withHttpOnlyFalse()) }`

### 2. `spring-permit-all`
* **Severity:** High | **CWE:** CWE-862 (Missing Authorization)
* **Risk:** Global `permitAll()` on sensitive root paths leaves endpoints open without authentication.
* **Bad Pattern:** `it.requestMatchers("/**").permitAll()`
* **Good Pattern:** Restrict `permitAll` to specific public paths (e.g. `/health`, `/login`) and require authentication by default: `anyRequest().authenticated()`.

### 3. `cors-any-host`
* **Severity:** Medium | **CWE:** CWE-942 (Overly Permissive Cross-Domain Whitelist)
* **Risk:** Configuring CORS to accept requests from any origin (`allowedOrigins("*")` or `anyHost()`).
* **Bad Pattern:** `cors { anyHost() }` or `configuration.allowedOrigins = listOf("*")`
* **Good Pattern:** Whitelist explicit domain origins: `cors { host("app.company.com", schemes = listOf("https")) }`

### 4. `actuator-exposed`
* **Severity:** Medium | **CWE:** CWE-200 (Exposure of Sensitive Information to an Unauthorized Actor)
* **Risk:** Exposing all Spring Actuator endpoints (`management.endpoints.web.exposure.include=*`) without authorization.
* **Bad Pattern:** `management.endpoints.web.exposure.include=*`
* **Good Pattern:** Expose only health metrics: `management.endpoints.web.exposure.include=health,info`

### 5. `sql-string-template`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Building SQL queries using Kotlin string templates inside JPA/JDBC calls.
* **Bad Pattern:** `entityManager.createQuery("SELECT u FROM User u WHERE u.email = '$email'")`
* **Good Pattern:** Use parameterized queries: `createQuery("SELECT u FROM User u WHERE u.email = :email").setParameter("email", email)`

### 6. `weak-crypto`
* **Severity:** High | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Using obsolete or broken ciphers and hashes (DES, 3DES, MD5, SHA-1).
* **Bad Pattern:** `MessageDigest.getInstance("MD5")`
* **Good Pattern:** Use `SHA-256`, `Argon2`, or `BCryptPasswordEncoder`.

### 7. `jwt-weak-hmac-secret`
* **Severity:** Critical | **CWE:** CWE-326 (Inadequate Encryption Strength)
* **Risk:** Hardcoded or low-entropy HMAC secret strings used to verify JWT tokens.
* **Bad Pattern:** `Keys.hmacShaKeyFor("secret".toByteArray())`
* **Good Pattern:** Load a cryptographically strong 256-bit key from environment or secrets vault.

### 8. `unvalidated-redirect`
* **Severity:** Medium | **CWE:** CWE-601 (URL Redirection to Untrusted Site)
* **Risk:** Redirecting to a URL received directly from an untrusted request parameter.
* **Bad Pattern:** `return "redirect:" + targetUrl`
* **Good Pattern:** Validate `targetUrl` against an internal domain whitelist before redirection.

### 9. `path-traversal-file`
* **Severity:** High | **CWE:** CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
* **Risk:** Constructing `File` instances directly from request parameters without canonical path validation.
* **Bad Pattern:** `val file = File(baseUploadDir, request.filename)`
* **Good Pattern:** Check that `file.canonicalPath.startsWith(baseUploadDir.canonicalPath)`.

### 10. `xxe-xml-factory`
* **Severity:** High | **CWE:** CWE-611 (Improper Restriction of XML External Entity Reference - XXE)
* **Risk:** Instantiating `DocumentBuilderFactory` without disabling external DTDs.
* **Bad Pattern:** `DocumentBuilderFactory.newInstance().newDocumentBuilder()`
* **Good Pattern:** `factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true)`
