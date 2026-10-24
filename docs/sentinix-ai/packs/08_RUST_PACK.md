# Sentinix AI: Rust Security Pack (7 Rules)

**Pack Identifier:** `rust`  
**Rule Count:** 7 Built-in Rules  
**Applicability Trigger:** Detection of Rust source files (`*.rs`) or `Cargo.toml`.  
**Focus:** Memory safety bypasses, arbitrary transmutation, command injection, unhandled panics, and FFI pointer lifetimes.

---

## Rule Catalogue

### 1. `rust/unsafe-block`
* **Severity:** Medium | **CWE:** CWE-242 (Use of Inherently Dangerous Function)
* **Risk:** Unannotated `unsafe` blocks bypass compiler memory safety, borrow checker, and thread safety guarantees.
* **Bad Pattern:** `unsafe { *ptr = 42; }`
* **Good Pattern:** Encapsulate in safe abstractions and provide explicit `// SAFETY:` explanatory comments.

### 2. `rust/transmute`
* **Severity:** High | **CWE:** CWE-89 / CWE-843 (Access of Resource Using Incompatible Type - Type Confusion)
* **Risk:** Using `std::mem::transmute` to bypass type safety can cause undefined behavior, memory corruption, and arbitrary execution.
* **Bad Pattern:** `let x: u32 = unsafe { std::mem::transmute(ptr) };`
* **Good Pattern:** Use safe casting (`as`), `bytemuck`, or safe conversion traits (`From`/`Into`).

### 3. `rust/command-injection`
* **Severity:** High | **CWE:** CWE-78 (Improper Neutralization of Special Elements used in an OS Command)
* **Risk:** Invoking `std::process::Command::new("sh").arg("-c").arg(userInput)` exposes commands to shell metacharacter expansion.
* **Bad Pattern:** `Command::new("sh").arg("-c").arg(format!("echo {}", input))`
* **Good Pattern:** Pass executable and arguments directly: `Command::new("echo").arg(input)` without shell wrappers.

### 4. `rust/raw-pointer-deref`
* **Severity:** High | **CWE:** CWE-476 (NULL Pointer Dereference)
* **Risk:** Dereferencing raw pointers (`*const T` or `*mut T`) without checking for null or validity.
* **Bad Pattern:** `unsafe { let val = *raw_ptr; }`
* **Good Pattern:** Check `if !raw_ptr.is_null()` or use `NonNull<T>`.

### 5. `rust/panic-in-production`
* **Severity:** Medium | **CWE:** CWE-703 (Improper Check or Handling of Exceptional Conditions)
* **Risk:** Explicit `panic!()`, `todo!()`, or `unimplemented!()` invocations in production API routes abort worker threads and can cause denial of service.
* **Bad Pattern:** `if invalid { panic!("fatal invalid state"); }`
* **Good Pattern:** Return a `Result<T, AppError>` and handle errors gracefully.

### 6. `rust/insecure-rand`
* **Severity:** Medium | **CWE:** CWE-338 (Use of Cryptographically Weak Pseudo-Random Number Generator)
* **Risk:** Using `rand::thread_rng()` for secret tokens or cryptographic key generation.
* **Bad Pattern:** `let token: u64 = rand::thread_rng().gen();`
* **Good Pattern:** Use `rand::rngs::OsRng` for cryptographic entropy.

### 7. `rust/ffi-cstring-as-ptr`
* **Severity:** High | **CWE:** CWE-416 (Use After Free)
* **Risk:** Converting a temporary `CString` directly via `.as_ptr()` (`CString::new(...).unwrap().as_ptr()`) causes the memory to be freed before the FFI call executes.
* **Bad Pattern:** `unsafe { ffi_call(CString::new(val).unwrap().as_ptr()); }`
* **Good Pattern:** Bind the `CString` to a local variable to preserve its lifetime: `let c_str = CString::new(val).unwrap(); unsafe { ffi_call(c_str.as_ptr()); }`
