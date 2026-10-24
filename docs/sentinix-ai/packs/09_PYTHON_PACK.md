# Sentinix AI: Python Security Pack (9 Rules)

**Pack Identifier:** `python`  
**Rule Count:** 9 Built-in Rules  
**Applicability Trigger:** Detection of Python source files (`*.py`), `requirements.txt`, `Pipfile`, or `pyproject.toml`.  
**Frameworks Covered:** Django, Flask, FastAPI, Celery, SQLAlchemy.

---

## Rule Catalogue

### 1. `python/sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing raw database queries using f-strings or `%` formatting (`cursor.execute(f"SELECT * FROM users WHERE id = '{user_id}'")`).
* **Bad Pattern:** `cursor.execute(f"SELECT * FROM users WHERE username = '{username}'")`
* **Good Pattern:** Use parameterized queries: `cursor.execute("SELECT * FROM users WHERE username = %s", (username,))`

### 2. `python/command-injection`
* **Severity:** Critical | **CWE:** CWE-78 (Improper Neutralization of Special Elements used in an OS Command)
* **Risk:** Executing shell commands using `os.system()` or `subprocess.Popen(..., shell=True)` with user input.
* **Bad Pattern:** `os.system("ping " + host)` or `subprocess.run(cmd, shell=True)`
* **Good Pattern:** Pass arguments as an array with `shell=False`: `subprocess.run(["ping", "-c", "1", host], check=True)`

### 3. `python/insecure-deserialization`
* **Severity:** Critical | **CWE:** CWE-502 (Deserialization of Untrusted Data)
* **Risk:** Loading untrusted data using `pickle.loads()` or `yaml.load()` without `SafeLoader` allows arbitrary code execution.
* **Bad Pattern:** `pickle.loads(user_data)` or `yaml.load(stream)`
* **Good Pattern:** Use `json.loads(user_data)` or `yaml.safe_load(stream)`.

### 4. `python/django-debug-enabled`
* **Severity:** High | **CWE:** CWE-489 (Active Debug Code)
* **Risk:** `DEBUG = True` in production Django settings exposes detailed stack traces and environment variables upon error.
* **Bad Pattern:** `DEBUG = True` in settings file without environment conditional
* **Good Pattern:** `DEBUG = os.getenv("DJANGO_DEBUG", "False").lower() in ("true", "1")`

### 5. `python/django-csrf-exempt`
* **Severity:** Medium | **CWE:** CWE-352 (Cross-Site Request Forgery - CSRF)
* **Risk:** Decorating sensitive state-changing views with `@csrf_exempt`.
* **Bad Pattern:** `@csrf_exempt def update_password(request):`
* **Good Pattern:** Keep CSRF validation active and ensure clients transmit the `csrftoken` header.

### 6. `python/weak-hash-md5`
* **Severity:** Medium | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Using `hashlib.md5()` or `hashlib.sha1()` for secure digests or password hashing.
* **Bad Pattern:** `hashlib.md5(password.encode()).hexdigest()`
* **Good Pattern:** Use `hashlib.sha256()` or `argon2-cffi`.

### 7. `python/ssrf-requests`
* **Severity:** High | **CWE:** CWE-918 (Server-Side Request Forgery - SSRF)
* **Risk:** Passing an unvalidated URL from a user request directly into `requests.get()`.
* **Bad Pattern:** `requests.get(request.GET.get("url"))`
* **Good Pattern:** Validate target IP addresses against private network ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.1).

### 8. `python/jinja-autoescape-disabled`
* **Severity:** High | **CWE:** CWE-79 (Improper Neutralization of Input During Web Page Generation - XSS)
* **Risk:** Initializing Jinja2 environment with `autoescape=False` disables HTML tag escaping.
* **Bad Pattern:** `Environment(loader=loader, autoescape=False)`
* **Good Pattern:** `Environment(loader=loader, autoescape=True)`

### 9. `python/insecure-tempfile-mktemp`
* **Severity:** Medium | **CWE:** CWE-377 (Insecure Temporary File)
* **Risk:** `tempfile.mktemp()` is deprecated and vulnerable to race conditions (symlink attacks).
* **Bad Pattern:** `tempfile.mktemp()`
* **Good Pattern:** Use `tempfile.NamedTemporaryFile()` or `tempfile.mkstemp()`.
