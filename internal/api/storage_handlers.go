package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

// HandleLocalStorageUpload handles local file uploads with HMAC pre-signed token verification
func (h *Handler) HandleLocalStorageUpload(c *gin.Context) {
	localProv, ok := h.Storage.(*storage.LocalProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Local storage provider is not active"})
		return
	}

	key := c.Query("key")
	expiresStr := c.Query("expires")
	sig := c.Query("sig")

	if key == "" || expiresStr == "" || sig == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing key, expires, or sig query parameter"})
		return
	}

	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expires parameter"})
		return
	}

	if err := localProv.VerifySignature("PUT", key, expiresUnix, sig); err != nil {
		if errors.Is(err, storage.ErrExpiredSignature) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Upload URL has expired"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or tampered upload signature"})
		return
	}

	// Bound the read to 10MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "File size exceeds maximum limit of 10MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	// Validate magic bytes
	if _, err := storage.ValidateMagicBytes(data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := localProv.SaveLocalFile(key, data); err != nil {
		if errors.Is(err, storage.ErrPathTraversal) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file key path"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "uploaded", "key": key})
}

// HandleLocalStorageDownload handles local file downloads with HMAC pre-signed token verification
func (h *Handler) HandleLocalStorageDownload(c *gin.Context) {
	localProv, ok := h.Storage.(*storage.LocalProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Local storage provider is not active"})
		return
	}

	key := c.Query("key")
	expiresStr := c.Query("expires")
	sig := c.Query("sig")

	if key == "" || expiresStr == "" || sig == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing key, expires, or sig query parameter"})
		return
	}

	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expires parameter"})
		return
	}

	if err := localProv.VerifySignature("GET", key, expiresUnix, sig); err != nil {
		if errors.Is(err, storage.ErrExpiredSignature) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Download URL has expired"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or tampered download signature"})
		return
	}

	filePath, err := localProv.GetLocalFilePath(key)
	if err != nil {
		if errors.Is(err, storage.ErrPathTraversal) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file key path"})
			return
		}
		if errors.Is(err, storage.ErrIsDirectory) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file key: target is a directory"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	filename := filepath.Base(key)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(filePath)
}
