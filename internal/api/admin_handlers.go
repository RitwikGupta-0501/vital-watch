package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListUsers returns a paginated list of all registered users across all roles (Admin-only).
func (h *Handler) ListUsers(c *gin.Context) {
	limit, offset := parsePagination(c)
	users, err := h.Repo.GetAllUsers(c.Request.Context(), limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error fetching users list", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	// Audit administrative access to user directory (HIPAA § 164.312(b))
	h.audit(c, audit.ActionViewUserDirectory, "user_directory", nil, nil, http.StatusOK, map[string]interface{}{
		"limit":  limit,
		"offset": offset,
		"count":  len(users),
	})

	c.JSON(http.StatusOK, gin.H{
		"data":   users,
		"limit":  limit,
		"offset": offset,
	})
}

// ToggleUserStatus activates or deactivates a user account (Admin-only).
// When deactivating, it automatically terminates all active sessions for the user.
func (h *Handler) ToggleUserStatus(c *gin.Context) {
	targetIDStr := c.Param("id")
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: is_active boolean is required"})
		return
	}

	// Self-Deactivation Guard: Prevent administrator from locking themselves out
	if callerIDVal, ok := c.Get("userID"); ok {
		if callerID, ok := callerIDVal.(uuid.UUID); ok && callerID == targetID && !req.IsActive {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Administrators cannot deactivate their own account"})
			return
		}
	}

	if err := h.Repo.UpdateUserActiveStatus(c.Request.Context(), targetID, req.IsActive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		slog.ErrorContext(c.Request.Context(), "Internal error updating user active status", "error", err, "target_user_id", targetID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user status"})
		return
	}

	// Immediate Session Invalidation: Terminate all active sessions on deactivation and clear memory cache
	h.InvalidateUserActiveCache(targetID)
	if !req.IsActive {
		if revokeErr := h.Repo.RevokeAllUserRefreshTokens(c.Request.Context(), targetID); revokeErr != nil {
			slog.WarnContext(c.Request.Context(), "Failed to revoke tokens on account deactivation", "error", revokeErr, "target_user_id", targetID)
		}
	}

	// Audit governance status change (HIPAA § 164.312(a)(2)(i))
	h.audit(c, audit.ActionToggleUserStatus, "user", &targetID, nil, http.StatusOK, map[string]interface{}{
		"target_user_id": targetID.String(),
		"is_active":      req.IsActive,
	})

	c.JSON(http.StatusOK, gin.H{
		"id":        targetID,
		"is_active": req.IsActive,
		"message":   "User status updated successfully",
	})
}

// GetComplianceAuditLogs retrieves system or patient-specific HIPAA audit trails.
// Administrators can view global or patient logs. Doctors must provide a patient_id
// with a verified clinical relationship.
func (h *Handler) GetComplianceAuditLogs(c *gin.Context) {
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)
	// Only administrators may query the full system audit trail.
	// Doctors may only query logs for their own patients, with a mandatory patient_id filter.
	if role != "doctor" && role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: HIPAA audit logs restricted to clinical and compliance roles"})
		return
	}

	limit := 50
	offset := 0
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	// Limit upper ceiling to prevent memory exhaustion DoS
	if limit > 100 {
		limit = 100
	}

	if oStr := c.Query("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var logs []models.PhiAuditLog
	var err error

	pStr := strings.TrimSpace(c.Query("patient_id"))

	if role == "doctor" {
		// Doctors MUST supply a patient_id and must have a clinical relationship.
		if pStr == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Doctors must supply a patient_id query parameter to filter audit logs"})
			return
		}
		pid, pErr := uuid.Parse(pStr)
		if pErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient_id"})
			return
		}
		doctorIDVal, ok := c.Get("userID")
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
			return
		}
		doctorID := doctorIDVal.(uuid.UUID)
		hasRel, relErr := h.Repo.HasDoctorPatientRelationship(c.Request.Context(), doctorID, pid)
		if relErr != nil {
			slog.ErrorContext(c.Request.Context(), "Error checking doctor-patient relationship for audit log access", "error", relErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify patient relationship"})
			return
		}
		if !hasRel {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you do not have a clinical relationship with this patient"})
			return
		}
		logs, err = h.Repo.GetAuditLogsByPatientID(c.Request.Context(), pid, limit, offset)
		// Audit the audit trail access itself
		h.audit(c, audit.ActionViewAuditLogs, "patient", nil, &pid, http.StatusOK, map[string]interface{}{"queried_by_role": role})
	} else {
		// Admins may query system-wide logs or filter by patient
		if pStr != "" {
			pid, pErr := uuid.Parse(pStr)
			if pErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient_id"})
				return
			}
			logs, err = h.Repo.GetAuditLogsByPatientID(c.Request.Context(), pid, limit, offset)
			h.audit(c, audit.ActionViewAuditLogs, "patient", nil, &pid, http.StatusOK, map[string]interface{}{"queried_by_role": role})
		} else {
			logs, err = h.Repo.GetAuditLogs(c.Request.Context(), limit, offset)
			h.audit(c, audit.ActionViewAuditLogs, "system", nil, nil, http.StatusOK, map[string]interface{}{"queried_by_role": role})
		}
	}

	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error fetching compliance audit logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve audit logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   logs,
		"limit":  limit,
		"offset": offset,
	})
}
