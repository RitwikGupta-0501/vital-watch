package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository/dbgen"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func generateInviteCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) CreateTenant(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant name is required"})
		return
	}

	var domain pgtype.Text
	if strings.TrimSpace(req.Domain) != "" {
		domain = pgtype.Text{String: strings.TrimSpace(req.Domain), Valid: true}
	}

	tenant, err := h.Repo.Queries().CreateTenant(c.Request.Context(), dbgen.CreateTenantParams{
		Name:   req.Name,
		Domain: domain,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant"})
		return
	}

	h.audit(c, audit.ActionPlatformAdminConfigUpdate, "tenant", nil, nil, http.StatusCreated, map[string]interface{}{"tenant_id": tenant.ID})
	c.JSON(http.StatusCreated, tenant)
}

func (h *Handler) CreateTenantInvite(c *gin.Context) {
	tenantID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	var req struct {
		Role      string `json:"role"`
		ExpiresIn int    `json:"expires_in_hours"` // Optional
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Role != "tenant_admin" && req.Role != "doctor" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role must be tenant_admin or doctor"})
		return
	}

	var expiresAt pgtype.Timestamptz
	if req.ExpiresIn > 0 {
		expiresAt = pgtype.Timestamptz{
			Time:  time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour),
			Valid: true,
		}
	}

	inviteCode := generateInviteCode()

	invite, err := h.Repo.Queries().CreateTenantInvite(c.Request.Context(), dbgen.CreateTenantInviteParams{
		TenantID:  tenantID,
		InviteCode: inviteCode,
		Role:      req.Role,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invite"})
		return
	}

	h.audit(c, audit.ActionPlatformAdminConfigUpdate, "tenant_invite", nil, nil, http.StatusCreated, map[string]interface{}{
		"tenant_id": tenantID,
		"role":      req.Role,
	})
	c.JSON(http.StatusCreated, invite)
}
