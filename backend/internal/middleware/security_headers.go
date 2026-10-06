package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeadersMiddleware sets standard HTTP response headers recommended by OWASP
// and required for HIPAA compliance to prevent clickjacking, MIME sniffing, and information disclosure.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking by forbidding embedding in frames/iframes
		c.Header("X-Frame-Options", "DENY")

		// Disable vulnerable legacy browser XSS filters in favor of Content-Security-Policy
		c.Header("X-XSS-Protection", "0")

		// Protect patient confidentiality by ensuring referrer headers do not leak PHI URLs to external origins
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Enforce HTTPS communication (HSTS: 1 year, includes subdomains)
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Restrict resource loading to self
		c.Header("Content-Security-Policy", "default-src 'self'")

		c.Next()
	}
}

// SensitiveCacheControlMiddleware sets strict no-cache headers on endpoints returning
// Protected Health Information (PHI) or authentication secrets to prevent caching on intermediate proxies or disk.
func SensitiveCacheControlMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")

		c.Next()
	}
}
