package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"pyntra/internal/database"
	"pyntra/internal/security/scope"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// EngagementHandler serves engagement CRUD and implements security.ScopeGuard,
// enforcing the active engagement's authorized target scope on tool calls.
type EngagementHandler struct {
	db     *database.DB
	logger *zap.Logger
}

// NewEngagementHandler constructs the handler.
func NewEngagementHandler(db *database.DB, logger *zap.Logger) *EngagementHandler {
	return &EngagementHandler{db: db, logger: logger}
}

// --- scope guard (implements security.ScopeGuard) --------------------------

// Check denies a tool call whose target falls outside the active engagement
// scope, or whose authorization window has lapsed. No active engagement, or an
// empty scope, permits everything (pre-engagement behavior).
func (h *EngagementHandler) Check(toolName string, args map[string]interface{}) error {
	if h == nil || h.db == nil {
		return nil
	}
	eng, err := h.db.GetActiveEngagement()
	if err != nil || eng == nil {
		return nil // fail-open on lookup error to avoid blocking legitimate work
	}
	// Authorization window.
	now := time.Now()
	if eng.AuthorizedFrom != nil && now.Before(*eng.AuthorizedFrom) {
		return fmt.Errorf("engagement %q authorization has not started yet", eng.Name)
	}
	if eng.AuthorizedTo != nil && now.After(*eng.AuthorizedTo) {
		return fmt.Errorf("engagement %q authorization window has ended", eng.Name)
	}
	// Dry-run: plan only, execute nothing.
	if eng.DryRun {
		return fmt.Errorf("engagement %q is in dry-run mode: tool execution is disabled (planning only)", eng.Name)
	}
	m := scope.NewMatcher(eng.Scope.Domains, eng.Scope.CIDRs, eng.Scope.URLs, eng.Scope.Exclusions)
	if m.IsEmpty() {
		return nil
	}
	targets := scope.ExtractTargets(args)
	if ok, reason := m.AllowAll(targets); !ok {
		return fmt.Errorf("%s (engagement %q)", reason, eng.Name)
	}
	return nil
}

// --- CRUD ------------------------------------------------------------------

// ListEngagements returns all engagements.
func (h *EngagementHandler) ListEngagements(c *gin.Context) {
	items, err := h.db.ListEngagements()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"engagements": items})
}

// GetActiveEngagement returns the active engagement (or null).
func (h *EngagementHandler) GetActiveEngagement(c *gin.Context) {
	e, err := h.db.GetActiveEngagement()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"engagement": e})
}

// CreateEngagement creates one.
func (h *EngagementHandler) CreateEngagement(c *gin.Context) {
	var e database.Engagement
	if err := c.ShouldBindJSON(&e); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(e.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	created, err := h.db.CreateEngagement(&e)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, created)
}

// UpdateEngagement updates one.
func (h *EngagementHandler) UpdateEngagement(c *gin.Context) {
	id := c.Param("id")
	var e database.Engagement
	if err := c.ShouldBindJSON(&e); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	e.ID = id
	if err := h.db.UpdateEngagement(&e); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// ActivateEngagement makes an engagement active (or clears with empty id body).
func (h *EngagementHandler) ActivateEngagement(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.SetActiveEngagement(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "active engagement set", "id": id})
}

// DeactivateEngagements clears the active engagement (removes scope enforcement).
func (h *EngagementHandler) DeactivateEngagements(c *gin.Context) {
	if err := h.db.SetActiveEngagement(""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "scope enforcement disabled"})
}

// DeleteEngagement removes one.
func (h *EngagementHandler) DeleteEngagement(c *gin.Context) {
	if err := h.db.DeleteEngagement(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
