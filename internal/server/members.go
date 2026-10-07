// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/gin-gonic/gin"
)

// --------------------------- Workspace admin ---------------------------
// Rutas /workspaces/:id/*: el workspace se resuelve por path, no por
// query (?workspace=), porque administran un workspace concreto.

type workspaceByIDResult struct {
	workspace db.Workspace
	role      string
}

// workspaceFromPath resuelve :id y valida que el caller sea miembro.
func (s *Server) workspaceFromPath(c *gin.Context, userID string) (workspaceByIDResult, bool) {
	ws, err := s.queries.GetWorkspaceByID(c, c.Param("id"))
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "workspace no encontrado")
		return workspaceByIDResult{}, false
	}
	member, err := s.queries.GetMembership(c, db.GetMembershipParams{UserID: userID, WorkspaceID: ws.ID})
	if err != nil {
		s.fail(c, http.StatusForbidden, "forbidden", "no perteneces a ese workspace")
		return workspaceByIDResult{}, false
	}
	return workspaceByIDResult{workspace: ws, role: member.Role}, true
}

func (s *Server) requireOwnerRole(c *gin.Context, role string) bool {
	if role != "owner" {
		s.fail(c, http.StatusForbidden, "forbidden", "se requiere rol owner")
		return false
	}
	return true
}

// handleGetWorkspace devuelve el workspace + el rol del caller.
func (s *Server) handleGetWorkspace(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": r.workspace.ID, "slug": r.workspace.Slug,
		"name": r.workspace.Name, "owner_id": r.workspace.OwnerID,
		"role": r.role,
	})
}

type workspacePatchRequest struct {
	Name string `json:"name" binding:"required,max=120"`
}

// handlePatchWorkspace renombra el workspace (solo owner).
func (s *Server) handlePatchWorkspace(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok || !s.requireOwnerRole(c, r.role) {
		return
	}
	var req workspacePatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := s.queries.UpdateWorkspaceName(c, db.UpdateWorkspaceNameParams{
		Name: req.Name, ID: r.workspace.ID,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo renombrar")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": r.workspace.ID, "name": req.Name})
}

// handleDeleteWorkspace elimina el workspace: índice, membresías y el
// árbol canónico de archivos (irreversible, solo owner).
func (s *Server) handleDeleteWorkspace(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok || !s.requireOwnerRole(c, r.role) {
		return
	}
	// orden FK-safe: primero las tablas hijas de docs, luego docs, luego
	// las del workspace. Sin esto, DELETE FROM docs falla con
	// foreign_keys(1) si hay tareas/versiones/shares/adjuntos.
	for _, stmt := range []string{
		`DELETE FROM doc_shares WHERE doc_id IN (SELECT id FROM docs WHERE workspace_id = ?)`,
		`DELETE FROM doc_versions WHERE doc_id IN (SELECT id FROM docs WHERE workspace_id = ?)`,
		`DELETE FROM tasks WHERE workspace_id = ?`,
		`DELETE FROM attachments WHERE workspace_id = ?`,
		`DELETE FROM backlinks WHERE src_doc_id IN (SELECT id FROM docs WHERE workspace_id = ?)`,
		`DELETE FROM backlinks WHERE dst_doc_id IN (SELECT id FROM docs WHERE workspace_id = ?)`,
		`DELETE FROM docs_fts WHERE doc_id IN (SELECT id FROM docs WHERE workspace_id = ?)`,
		`DELETE FROM saved_views WHERE workspace_id = ?`,
		`DELETE FROM change_log WHERE workspace_id = ?`,
		`DELETE FROM sync_commands WHERE workspace_id = ?`,
		`DELETE FROM webhooks WHERE workspace_id = ?`,
		`DELETE FROM audit_log WHERE workspace_id = ?`,
		`DELETE FROM docs WHERE workspace_id = ?`,
		`DELETE FROM memberships WHERE workspace_id = ?`,
		`DELETE FROM workspaces WHERE id = ?`,
	} {
		if _, err := s.conn.ExecContext(c, stmt, r.workspace.ID); err != nil {
			s.logger.Warn("delete workspace", "stmt", stmt, "err", err)
			s.fail(c, http.StatusInternalServerError, "internal", "no se pudo eliminar el workspace")
			return
		}
	}
	// el slug ya está validado al crear; se usa como segmento simple
	slug := r.workspace.Slug
	if strings.ContainsAny(slug, `/\`) || slug == ".." || slug == "." {
		s.logger.Error("slug de workspace inseguro al borrar", "slug", slug)
	} else if err := os.RemoveAll(filepath.Join(s.cfg.DataDir, "workspaces", slug)); err != nil {
		s.logger.Warn("borrando árbol canónico del workspace", "slug", slug, "err", err)
	}
	c.JSON(http.StatusNoContent, nil)
}

// ------------------------------ Members -------------------------------

// handleListMembers lista miembros del workspace (cualquier miembro lee).
func (s *Server) handleListMembers(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok {
		return
	}
	rows, err := s.queries.ListMembersByWorkspace(c, r.workspace.ID)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "error de base de datos")
		return
	}
	if rows == nil {
		rows = []db.ListMembersByWorkspaceRow{}
	}
	c.JSON(http.StatusOK, gin.H{"members": rows})
}

type memberRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required,oneof=owner editor viewer"`
}

// handleAddMember añade un usuario existente al workspace por email.
// Kora Hub no envía invitaciones por correo (un binario, sin SMTP en el
// MVP): el usuario invitado debe haberse registrado previamente.
func (s *Server) handleAddMember(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok || !s.requireOwnerRole(c, r.role) {
		return
	}
	var req memberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user, err := s.queries.GetUserByEmail(c, req.Email)
	if err != nil {
		s.fail(c, http.StatusNotFound, "user_not_found", "no existe un usuario con ese email")
		return
	}
	if _, err := s.queries.GetMembership(c, db.GetMembershipParams{
		UserID: user.ID, WorkspaceID: r.workspace.ID,
	}); err == nil {
		s.fail(c, http.StatusConflict, "already_member", "ese usuario ya es miembro")
		return
	}
	if err := s.queries.AddMembership(c, db.AddMembershipParams{
		UserID: user.ID, WorkspaceID: r.workspace.ID, Role: req.Role,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo añadir el miembro")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"user_id": user.ID, "email": user.Email,
		"display_name": user.DisplayName, "role": req.Role,
	})
}

type memberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=owner editor viewer"`
}

// handleUpdateMemberRole cambia el rol de un miembro (solo owner).
// Protección: el último owner no puede perder su rol.
func (s *Server) handleUpdateMemberRole(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok || !s.requireOwnerRole(c, r.role) {
		return
	}
	target := c.Param("uid")
	member, err := s.queries.GetMembership(c, db.GetMembershipParams{
		UserID: target, WorkspaceID: r.workspace.ID,
	})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "miembro no encontrado")
		return
	}
	var req memberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if member.Role == "owner" && req.Role != "owner" && !s.otherOwnerExists(c, r.workspace.ID, target) {
		s.fail(c, http.StatusConflict, "last_owner", "no puede quedar un workspace sin owner")
		return
	}
	if err := s.queries.UpdateMembershipRole(c, db.UpdateMembershipRoleParams{
		Role: req.Role, UserID: target, WorkspaceID: r.workspace.ID,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo cambiar el rol")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": target, "role": req.Role})
}

// handleRemoveMember saca a un miembro del workspace (solo owner).
// Un owner puede eliminarse a sí mismo si queda otro owner.
func (s *Server) handleRemoveMember(c *gin.Context) {
	r, ok := s.workspaceFromPath(c, c.GetString("user_id"))
	if !ok || !s.requireOwnerRole(c, r.role) {
		return
	}
	target := c.Param("uid")
	member, err := s.queries.GetMembership(c, db.GetMembershipParams{
		UserID: target, WorkspaceID: r.workspace.ID,
	})
	if err != nil {
		s.fail(c, http.StatusNotFound, "not_found", "miembro no encontrado")
		return
	}
	if member.Role == "owner" && !s.otherOwnerExists(c, r.workspace.ID, target) {
		s.fail(c, http.StatusConflict, "last_owner", "no puede quedar un workspace sin owner")
		return
	}
	if err := s.queries.RemoveMembership(c, db.RemoveMembershipParams{
		UserID: target, WorkspaceID: r.workspace.ID,
	}); err != nil {
		s.fail(c, http.StatusInternalServerError, "internal", "no se pudo eliminar el miembro")
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// otherOwnerExists: hay otro owner distinto de excludeUserID.
func (s *Server) otherOwnerExists(c *gin.Context, workspaceID, excludeUserID string) bool {
	members, err := s.queries.ListMembersByWorkspace(c, workspaceID)
	if err != nil {
		return false
	}
	for _, m := range members {
		if m.Role == "owner" && m.UserID != excludeUserID {
			return true
		}
	}
	return false
}
