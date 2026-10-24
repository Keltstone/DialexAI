# Sentinix AI: Node.js & React Security Pack (9 Rules)

**Pack Identifier:** `node-react`  
**Rule Count:** 9 Built-in Rules  
**Applicability Trigger:** Detection of JavaScript / TypeScript files (`*.js`, `*.jsx`, `*.ts`, `*.tsx`) or `package.json`.  
**Frameworks Covered:** Express, Fastify, Next.js, React, Node.js runtime.

---

## Rule Catalogue

### 1. `react/dangerously-set-inner-html`
* **Severity:** High | **CWE:** CWE-79 (Improper Neutralization of Input During Web Page Generation - XSS)
* **Risk:** Using `dangerouslySetInnerHTML` bypasses React's automatic string escaping and introduces Cross-Site Scripting (XSS).
* **Bad Pattern:** `<div dangerouslySetInnerHTML={{ __html: userBio }} />`
* **Good Pattern:** Render plain text children `<div>{userBio}</div>` or sanitize with DOMPurify: `DOMPurify.sanitize(userBio)`.

### 2. `node/eval-injection`
* **Severity:** Critical | **CWE:** CWE-95 (Improper Neutralization of Directives in Dynamically Evaluated Code)
* **Risk:** Passing dynamic input into `eval()` or `new Function()` allows arbitrary code execution on the server or client.
* **Bad Pattern:** `eval("var res = " + req.body.formula)`
* **Good Pattern:** Use safe mathematical or expression parsers (e.g. `expr-eval`).

### 3. `node/command-injection`
* **Severity:** Critical | **CWE:** CWE-78 (Improper Neutralization of Special Elements used in an OS Command)
* **Risk:** Invoking `child_process.exec()` with user string concatenation triggers shell metacharacter injection.
* **Bad Pattern:** `exec("ping -c 1 " + req.query.host)`
* **Good Pattern:** Use `child_process.execFile()` or `spawn()` with an array of arguments without a shell wrapper.

### 4. `node/sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing database queries using JavaScript template strings.
* **Bad Pattern:** `db.query(\`SELECT * FROM users WHERE email = '${req.body.email}'\`)`
* **Good Pattern:** Use parameterized queries: `db.query('SELECT * FROM users WHERE email = $1', [req.body.email])`.

### 5. `node/cors-wildcard`
* **Severity:** Medium | **CWE:** CWE-942 (Overly Permissive Cross-Domain Whitelist)
* **Risk:** Setting `origin: "*"` with `credentials: true` in Express CORS middleware.
* **Bad Pattern:** `app.use(cors({ origin: "*" }))`
* **Good Pattern:** Restrict origin to explicit trusted domains: `app.use(cors({ origin: "https://app.corp.internal" }))`.

### 6. `node/path-traversal-join`
* **Severity:** High | **CWE:** CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
* **Risk:** Reading files using `path.join(uploadDir, req.query.file)` without verifying that the resolved path is inside `uploadDir`.
* **Bad Pattern:** `fs.readFileSync(path.join("/uploads", req.query.file))`
* **Good Pattern:** Check that `path.resolve(targetPath).startsWith(path.resolve(uploadDir))`.

### 7. `node/jwt-none-algorithm`
* **Severity:** Critical | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Configuring JWT verification to accept the `"none"` algorithm allows signature-less token forgery.
* **Bad Pattern:** `jwt.verify(token, secret, { algorithms: ["HS256", "none"] })`
* **Good Pattern:** Restrict to secure algorithms: `{ algorithms: ["HS256"] }`.

### 8. `react/href-javascript`
* **Severity:** High | **CWE:** CWE-79 (Improper Neutralization of Input During Web Page Generation - XSS)
* **Risk:** Setting `href="javascript:..."` or accepting unvalidated protocol schemes in `<a>` links.
* **Bad Pattern:** `<a href={userWebsite}>Visit Site</a>` where `userWebsite` can be `javascript:alert(1)`
* **Good Pattern:** Verify scheme matches `https://` or `http://` before setting href.

### 9. `node/insecure-cookie-flag`
* **Severity:** Medium | **CWE:** CWE-614 (Sensitive Cookie in HTTPS Session Without 'Secure' Attribute)
* **Risk:** Issuing session cookies with `httpOnly: false` or `secure: false`.
* **Bad Pattern:** `res.cookie("session", id, { httpOnly: false, secure: false })`
* **Good Pattern:** `res.cookie("session", id, { httpOnly: true, secure: true, sameSite: "strict" })`
