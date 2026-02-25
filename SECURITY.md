# Security Policy

## Security Features

Lanpaper implements industry-standard security practices following OWASP, W3C, and modern web security guidelines (2026).

### 🛡️ Content Security Policy (CSP)

**Strict CSP with Nonce-based Inline Script Protection**

Following [OWASP CSP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html) and [W3C CSP Level 3](https://www.w3.org/TR/CSP3/):

```
Content-Security-Policy:
  default-src 'none';
  script-src 'self' 'nonce-{random}';
  style-src 'self' 'nonce-{random}';
  img-src 'self' https: data: blob:;
  media-src 'self' https: data: blob:;
  connect-src 'self';
  font-src 'self';
  manifest-src 'self';
  worker-src 'self';
  object-src 'none';
  base-uri 'self';
  form-action 'self';
  frame-ancestors 'none';
  upgrade-insecure-requests;
  block-all-mixed-content;
```

**Key Security Features:**
- ✅ **No `unsafe-inline`** - All inline scripts require cryptographic nonce
- ✅ **No `unsafe-eval`** - Prevents eval-based XSS attacks
- ✅ **Cryptographically secure nonce** - 128 bits of entropy, unique per request
- ✅ **`default-src 'none'`** - Explicit allowlists for each resource type
- ✅ **`object-src 'none'`** - Blocks Flash, Java applets, and plugins
- ✅ **`frame-ancestors 'none'`** - Prevents clickjacking attacks
- ✅ **`upgrade-insecure-requests`** - Auto-upgrades HTTP to HTTPS
- ✅ **`block-all-mixed-content`** - Prevents mixed content vulnerabilities

---

### 🔒 Security Headers

#### HSTS (HTTP Strict Transport Security)
```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
```
- Forces HTTPS for 2 years
- Includes all subdomains
- Ready for HSTS preload list

#### CORP, COOP, COEP (Cross-Origin Policies)
```
Cross-Origin-Resource-Policy: same-origin
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
```
- **CORP**: Prevents cross-origin resource leaks (Spectre mitigation)
- **COOP**: Isolates browsing context from cross-origin windows
- **COEP**: Requires explicit CORS for cross-origin resources

#### Other Security Headers
```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: geolocation=(), microphone=(), camera=(), payment=(), usb=()
X-Download-Options: noopen
```

---

### 🔐 Service Worker Security

Following [Service Worker Security Best Practices 2026](https://www.zeepalm.com/blog/service-worker-security-best-practices-2024-guide):

#### Secure Registration
- ✅ **HTTPS-only** - Service workers require secure context
- ✅ **Same-origin only** - Only fetches from same origin
- ✅ **Credentials policy** - `credentials: 'same-origin'` on all fetches

#### Content Type Validation
```javascript
const safeTypes = ['text/', 'application/json', 'application/javascript', 'image/', 'font/'];
if (safeTypes.some(type => contentType.startsWith(type))) {
  // Cache only safe content types
}
```

#### Cache Security
- ✅ **Cache size limits** - Runtime cache limited to 50 items
- ✅ **Cache expiration** - Automatic cleanup of 24-hour-old entries
- ✅ **Timestamp tracking** - Each cached response includes cache time
- ✅ **Graceful degradation** - Missing files don't break installation

#### Stale-While-Revalidate Strategy
- Serves cached content immediately
- Updates cache in background
- Ensures fresh content without blocking

---

### 🚨 Rate Limiting

**Token Bucket Algorithm** with per-IP tracking:

- **Public endpoints**: 60 requests/minute, burst of 10
- **Admin endpoints**: Stricter limits via upload middleware
- **API endpoints**: Separate rate limiting

---

### 🔑 Authentication

- **Admin panel** requires authentication via middleware
- **Session management** with secure cookies (when implemented)
- **Password hashing** using bcrypt or Argon2 (when implemented)

---

## Security Checklist

### ✅ Implemented

- [x] Strict Content Security Policy with nonces
- [x] HSTS with 2-year max-age and preload
- [x] Cross-Origin Resource Policy (CORP)
- [x] Cross-Origin Opener Policy (COOP)
- [x] Cross-Origin Embedder Policy (COEP)
- [x] X-Content-Type-Options: nosniff
- [x] X-Frame-Options: DENY
- [x] Referrer-Policy: strict-origin-when-cross-origin
- [x] Permissions-Policy (feature restrictions)
- [x] Service Worker security (HTTPS-only, same-origin, content validation)
- [x] Rate limiting (token bucket algorithm)
- [x] Secure random nonce generation (crypto/rand)
- [x] Cache size and age limits in Service Worker
- [x] Input validation for file uploads
- [x] MIME type validation

### 🔄 Recommended for Production

- [ ] **CSP Reporting** - Add `report-uri` or `report-to` directive
- [ ] **Subresource Integrity (SRI)** - Add integrity attributes to external scripts
- [ ] **Security.txt** - Add `/.well-known/security.txt` for vulnerability disclosure
- [ ] **Certificate Transparency** - Monitor CT logs for unauthorized certificates
- [ ] **Security audits** - Regular penetration testing
- [ ] **Dependency scanning** - Use `go mod verify` and vulnerability scanners
- [ ] **HTTPS enforcement** - Use Let's Encrypt with auto-renewal
- [ ] **Session security** - Implement secure session tokens with rotation
- [ ] **SQL injection protection** - Use parameterized queries (if SQL added)
- [ ] **XSS sanitization** - Escape user input in templates

---

## CSP Implementation Notes

### Using Nonce in Templates

To use the generated nonce in your HTML templates:

```go
// In your handler:
nonce := w.Header().Get("X-Nonce")

// In your template:
<script nonce="{{.Nonce}}">
  // Your inline script
</script>

<style nonce="{{.Nonce}}">
  /* Your inline styles */
</style>
```

### External Scripts

For external scripts (CDNs), add them to CSP:

```go
// Example for adding Google Analytics:
"script-src 'self' 'nonce-" + nonce + "' https://www.googletagmanager.com;"
```

### CSP Testing

1. **Report-Only Mode** (for testing):
   ```go
   h.Set("Content-Security-Policy-Report-Only", buildCSP(nonce))
   ```

2. **CSP Evaluator** - Use [Google CSP Evaluator](https://csp-evaluator.withgoogle.com/)

3. **Browser DevTools** - Check console for CSP violations

---

## Service Worker Updates

### Manual Cache Clear

```javascript
// From browser console:
navigator.serviceWorker.controller.postMessage({ action: 'clearCache' });
```

### Force Update

```javascript
navigator.serviceWorker.getRegistration().then(reg => {
  reg.update(); // Check for updates
});
```

### Unregister (for debugging)

```javascript
navigator.serviceWorker.getRegistration().then(reg => {
  reg.unregister();
  location.reload();
});
```

---

## Security Considerations

### File Upload Security

1. **MIME type validation** - Server validates Content-Type
2. **File extension whitelist** - Only allow safe extensions
3. **File size limits** - Prevent DoS via large uploads
4. **Virus scanning** - Consider adding ClamAV integration
5. **Storage isolation** - Uploaded files stored outside web root when possible

### External Image URLs

- **URL validation** - Ensure HTTPS-only for external images
- **SSRF protection** - Validate and sanitize external URLs
- **Timeout limits** - Prevent slow-read attacks
- **Content-Type verification** - Validate response headers

---

## Reporting Security Issues

Please report security vulnerabilities via:

- **Email**: [your-security-email@example.com]
- **GitHub Security Advisory**: Use "Report a vulnerability" button
- **PGP Key**: [Link to PGP public key if available]

**Do NOT** open public issues for security vulnerabilities.

---

## References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [OWASP CSP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html)
- [W3C CSP Level 3](https://www.w3.org/TR/CSP3/)
- [MDN Web Security](https://developer.mozilla.org/en-US/docs/Web/Security)
- [Service Worker Security](https://www.zeepalm.com/blog/service-worker-security-best-practices-2024-guide)
- [Spectre & Meltdown Mitigations](https://web.dev/articles/spectre)

---

## License

Security features are part of Lanpaper and licensed under the same terms as the main project.
