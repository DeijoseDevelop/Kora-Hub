// Copyright (C) 2026 Deijose <tech@deijose.dev>
// SPDX-License-Identifier: AGPL-3.0-only
package server

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/DeijoseDevelop/Kora-Hub/internal/db"
	"github.com/DeijoseDevelop/Kora-Hub/internal/mcp"
	"github.com/gin-gonic/gin"
)

// resolveWorkspace replica workspaceOf sin gin: mismo orden (?workspace=
// o primer workspace del usuario) y misma validacion de membresia. El
// endpoint MCP la usa como resolver de tenant para cada tools/call.
func (s *Server) resolveWorkspace(ctx context.Context, want, userID string) (mcp.Workspace, error) {
	if want == "" {
		rows, err := s.queries.ListWorkspacesByUser(ctx, userID)
		if err != nil {
			return mcp.Workspace{}, err
		}
		if len(rows) == 0 {
			return mcp.Workspace{}, errors.New("no_workspace")
		}
		return mcp.Workspace{ID: rows[0].ID, Slug: rows[0].Slug, Role: rows[0].Role}, nil
	}
	member, err := s.queries.GetMembership(ctx, db.GetMembershipParams{UserID: userID, WorkspaceID: want})
	if err != nil {
		return mcp.Workspace{}, errors.New("forbidden")
	}
	ws, err := s.queries.GetWorkspaceByID(ctx, want)
	if err != nil {
		return mcp.Workspace{}, errors.New("not_found")
	}
	return mcp.Workspace{ID: ws.ID, Slug: ws.Slug, Role: member.Role}, nil
}

// handleMCP es el transporte streamable HTTP de MCP: POST /api/v1/mcp
// con el mismo Bearer y limite de 1 MB por peticion.
func (s *Server) handleMCP(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		s.fail(c, http.StatusBadRequest, "bad_request", "cuerpo ilegible")
		return
	}
	resp, status := s.mcp.Handle(c.Request.Context(), c.GetString("user_id"), c.Query("workspace"), body)
	if status == http.StatusAccepted {
		c.Status(http.StatusAccepted)
		return
	}
	c.JSON(status, resp)
}
