package work

import (
	"net/http"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/issue"
)

type issueCommentResponse struct {
	ID              string     `json:"id"`
	IssueID         string     `json:"issue_id"`
	AuthorKind      string     `json:"author_kind"`
	AuthorID        string     `json:"author_id"`
	Body            string     `json:"body"`
	SourceTaskID    *string    `json:"source_task_id,omitempty"`
	SourceTaskRunID *string    `json:"source_task_run_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	EditedAt        *time.Time `json:"edited_at,omitempty"`
}

type issueCommentListResponse struct {
	Comments []issueCommentResponse `json:"comments"`
	Total    int                    `json:"total"`
}

type issueCommentRequest struct {
	Body string `json:"body"`
}

func issueCommentToResponse(comment model.IssueComment) issueCommentResponse {
	return issueCommentResponse{
		ID:              comment.ID,
		IssueID:         comment.IssueID,
		AuthorKind:      comment.AuthorKind,
		AuthorID:        comment.AuthorID,
		Body:            comment.Body,
		SourceTaskID:    comment.SourceTaskID,
		SourceTaskRunID: comment.SourceTaskRunID,
		CreatedAt:       comment.CreatedAt,
		EditedAt:        comment.EditedAt,
	}
}

// resolveCommentIssue authorizes a comment request through its issue, which is
// what owns the team. A comment carries no team of its own, so every route
// starts here.
func (h *Handler) resolveCommentIssue(w http.ResponseWriter, r *http.Request) (userID, teamID, issueID string, ok bool) {
	userID, teamID, ok = h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return "", "", "", false
	}
	if !httputil.RequireStore(w, h.cfg.IssueComments, "comments not configured") {
		return "", "", "", false
	}
	issueID, ok = httputil.PathValue(w, r, "issue_id")
	if !ok {
		return "", "", "", false
	}
	found, err := h.cfg.Issues.GetIssue(r.Context(), issueID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_comment_issue", "issue_id", issueID)
		return "", "", "", false
	}
	if found == nil || found.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return "", "", "", false
	}
	return userID, teamID, issueID, true
}

func (h *Handler) listIssueCommentsHandler(w http.ResponseWriter, r *http.Request) {
	_, _, issueID, ok := h.resolveCommentIssue(w, r)
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.BulkPageDefault, httputil.BulkPageMax)
	list, total, err := h.issueService().ListComments(r.Context(), issueID, limit, offset)
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_issue_comments", "issue_id", issueID)
		return
	}
	out := make([]issueCommentResponse, len(list))
	for i := range list {
		out[i] = issueCommentToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, issueCommentListResponse{Comments: out, Total: total})
}

func (h *Handler) createIssueCommentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, issueID, ok := h.resolveCommentIssue(w, r)
	if !ok {
		return
	}
	if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionCommentIssue); !ok {
		return
	}
	var req issueCommentRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	created, err := h.issueService().CreateComment(r.Context(), issue.CreateCommentCmd{
		IssueID:    issueID,
		AuthorKind: model.IssueCommentAuthorUser,
		AuthorID:   userID,
		Body:       req.Body,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_issue_comment", "issue_id", issueID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, issueCommentToResponse(*created))
}

func (h *Handler) patchIssueCommentHandler(w http.ResponseWriter, r *http.Request) {
	userID, _, issueID, ok := h.resolveCommentIssue(w, r)
	if !ok {
		return
	}
	commentID, ok := httputil.PathValue(w, r, "comment_id")
	if !ok {
		return
	}
	var req issueCommentRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	updated, err := h.issueService().UpdateComment(r.Context(), issue.UpdateCommentCmd{
		IssueID:   issueID,
		CommentID: commentID,
		UserID:    userID,
		Body:      req.Body,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "patch_issue_comment", "comment_id", commentID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, issueCommentToResponse(*updated))
}

func (h *Handler) deleteIssueCommentHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, issueID, ok := h.resolveCommentIssue(w, r)
	if !ok {
		return
	}
	commentID, ok := httputil.PathValue(w, r, "comment_id")
	if !ok {
		return
	}
	// Moderation is checked without writing a response: deleting your own
	// comment needs no permission, so a member without it is not being refused.
	err := h.issueService().DeleteComment(r.Context(), issue.DeleteCommentCmd{
		IssueID:     issueID,
		CommentID:   commentID,
		UserID:      userID,
		CanModerate: h.guard().MemberAllows(r.Context(), userID, teamID, access.ActionModerateIssueComments),
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "delete_issue_comment", "comment_id", commentID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
