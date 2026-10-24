# Sentinix AI: .NET & C# Security Pack (8 Rules)

**Pack Identifier:** `dotnet`  
**Rule Count:** 8 Built-in Rules  
**Applicability Trigger:** Detection of C# source files (`*.cs`), project files (`*.csproj`), or `appsettings*.json`.  
**Standards Alignment:** Microsoft Security Development Lifecycle (SDL), OWASP Top 10 for .NET.

---

## Rule Catalogue

### 1. `dotnet/sql-injection`
* **Severity:** High | **CWE:** CWE-89 (Improper Neutralization of Special Elements used in an SQL Command)
* **Risk:** Constructing SQL commands via C# string interpolation (`SqlCommand($"SELECT * FROM Users WHERE Id = '{id}'")`).
* **Bad Pattern:** `new SqlCommand($"SELECT * FROM Users WHERE Email = '{email}'", conn)`
* **Good Pattern:** Use parameterized queries: `cmd.Parameters.AddWithValue("@email", email)`.

### 2. `dotnet/insecure-deserialization`
* **Severity:** Critical | **CWE:** CWE-502 (Deserialization of Untrusted Data)
* **Risk:** Using `BinaryFormatter`, `NetDataContractSerializer`, or JSON.NET `TypeNameHandling.All` allows arbitrary remote code execution.
* **Bad Pattern:** `new BinaryFormatter().Deserialize(stream)`
* **Good Pattern:** Use `System.Text.Json.JsonSerializer` with strongly typed DTOs.

### 3. `dotnet/weak-crypto-md5-sha1`
* **Severity:** Medium | **CWE:** CWE-327 (Use of a Broken or Risky Cryptographic Algorithm)
* **Risk:** Instantiating `MD5.Create()` or `SHA1.Create()` for password or integrity hashing.
* **Bad Pattern:** `MD5.Create().ComputeHash(bytes)`
* **Good Pattern:** Use `SHA256.Create()` or `HMACSHA256`.

### 4. `dotnet/hardcoded-connection-string`
* **Severity:** High | **CWE:** CWE-798 (Use of Hard-coded Credentials)
* **Risk:** Hardcoding database credentials in connection strings in source code or unencrypted configuration.
* **Bad Pattern:** `"Server=db.corp;Database=prod;User Id=sa;Password=Secret123;"`
* **Good Pattern:** Store connection strings in Azure Key Vault or AWS Secrets Manager.

### 5. `dotnet/xxe-xml-document`
* **Severity:** High | **CWE:** CWE-611 (Improper Restriction of XML External Entity Reference - XXE)
* **Risk:** Parsing XML using `XmlDocument` without setting `XmlResolver = null`.
* **Bad Pattern:** `var doc = new XmlDocument(); doc.LoadXml(xml)` (with resolver enabled)
* **Good Pattern:** `var doc = new XmlDocument { XmlResolver = null }; doc.LoadXml(xml)`

### 6. `dotnet/antiforgery-disabled`
* **Severity:** High | **CWE:** CWE-352 (Cross-Site Request Forgery - CSRF)
* **Risk:** Explicitly disabling anti-forgery token validation using `[IgnoreAntiforgeryToken]`.
* **Bad Pattern:** `[HttpPost, IgnoreAntiforgeryToken] public IActionResult UpdateProfile(...)`
* **Good Pattern:** Apply `[ValidateAntiForgeryToken]` or `[AutoValidateAntiforgeryToken]`.

### 7. `dotnet/ldap-injection`
* **Severity:** High | **CWE:** CWE-90 (Improper Neutralization of Special Elements used in an LDAP Query)
* **Risk:** Building LDAP search filters using raw string interpolation.
* **Bad Pattern:** `var filter = $"(uid={username})"`
* **Good Pattern:** Sanitize and escape special characters (`*`, `(`, `)`, `\`, `NUL`) before query execution.

### 8. `dotnet/open-redirect`
* **Severity:** Medium | **CWE:** CWE-601 (URL Redirection to Untrusted Site)
* **Risk:** Calling `Redirect(returnUrl)` without verifying that the URL is local.
* **Bad Pattern:** `return Redirect(returnUrl);`
* **Good Pattern:** `if (Url.IsLocalUrl(returnUrl)) return Redirect(returnUrl); return RedirectToAction("Index", "Home");`
