package issue

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

var (
	ErrCommentsNotConfigured = apierr.New(apierr.KindNotConfigured, "comments not configured")
	ErrCommentBodyRequired   = apierr.New(apierr.KindInvalid, "comment body required")
	ErrCommentTooLong        = apierr.New(apierr.KindInvalid, "comment too long")
	ErrCommentNotFound       = apierr.New(apierr.KindNotFound, "comment not found")
	ErrCommentNotEditable    = apierr.New(apierr.KindForbidden, "comment not editable")
)

// CommentBodyLimit bounds a comment body in bytes.
//
// A comment is a statement about an issue, not a place to paste a run's output:
// long content belongs in an artifact, and a comment points at one. The limit
// also bounds the size of a thread response, which is otherwise unbounded in
// the number of comments times their length.
const CommentBodyLimit = 16 * 1024

type CreateCommentCmd struct {
	IssueID         string
	AuthorKind      string
	AuthorID        string
	Body            string
	SourceTaskID    *string
	SourceTaskRunID *string
}

type UpdateCommentCmd struct {
	IssueID   string
	CommentID string
	UserID    string
	Body      string
	// CanModerate is true when the caller may edit or delete a comment they did
	// not write. Only deletion honors it; see UpdateComment.
	CanModerate bool
}

type DeleteCommentCmd struct {
	IssueID     string
	CommentID   string
	UserID      string
	CanModerate bool
}

// CreateComment appends a comment to an issue. The caller is responsible for
// having authorized the issue's team.
func (s *IssueService) CreateComment(ctx context.Context, cmd CreateCommentCmd) (*model.IssueComment, error) {
	if s.Comments == nil {
		return nil, ErrCommentsNotConfigured
	}
	body, err := validateCommentBody(cmd.Body)
	if err != nil {
		return nil, err
	}
	kind := cmd.AuthorKind
	if kind == "" {
		kind = model.IssueCommentAuthorUser
	}
	return s.Comments.CreateIssueComment(ctx, model.CreateIssueCommentInput{
		IssueID:         cmd.IssueID,
		AuthorKind:      kind,
		AuthorID:        cmd.AuthorID,
		Body:            body,
		SourceTaskID:    cmd.SourceTaskID,
		SourceTaskRunID: cmd.SourceTaskRunID,
	})
}

// ListComments returns an issue's thread, oldest first.
func (s *IssueService) ListComments(ctx context.Context, issueID string, limit, offset int) ([]model.IssueComment, int, error) {
	if s.Comments == nil {
		return nil, 0, ErrCommentsNotConfigured
	}
	return s.Comments.ListIssueComments(ctx, issueID, limit, offset)
}

// UpdateComment replaces a comment's body.
//
// Only the person who wrote a comment may edit it. Moderation permits deletion,
// not rewriting: an edit puts words in another person's mouth, and an agent or
// system comment is the record of what a run reported — a record anyone can
// rewrite is not one.
func (s *IssueService) UpdateComment(ctx context.Context, cmd UpdateCommentCmd) (*model.IssueComment, error) {
	comment, err := s.loadComment(ctx, cmd.IssueID, cmd.CommentID)
	if err != nil {
		return nil, err
	}
	if comment.AuthorKind != model.IssueCommentAuthorUser || comment.AuthorID != cmd.UserID {
		return nil, ErrCommentNotEditable
	}
	body, err := validateCommentBody(cmd.Body)
	if err != nil {
		return nil, err
	}
	updated, err := s.Comments.UpdateIssueComment(ctx, cmd.CommentID, body)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrCommentNotFound
	}
	return updated, nil
}

// DeleteComment removes a comment. The author may delete their own; a moderator
// may delete any comment on the issue, including one an agent wrote.
func (s *IssueService) DeleteComment(ctx context.Context, cmd DeleteCommentCmd) error {
	comment, err := s.loadComment(ctx, cmd.IssueID, cmd.CommentID)
	if err != nil {
		return err
	}
	own := comment.AuthorKind == model.IssueCommentAuthorUser && comment.AuthorID == cmd.UserID
	if !own && !cmd.CanModerate {
		return ErrCommentNotEditable
	}
	return s.Comments.DeleteIssueComment(ctx, cmd.CommentID)
}

// loadComment resolves a comment and verifies it belongs to the issue the
// caller was authorized against. A comment ID from another issue is not found,
// not a successful write to somewhere else.
func (s *IssueService) loadComment(ctx context.Context, issueID, commentID string) (*model.IssueComment, error) {
	if s.Comments == nil {
		return nil, ErrCommentsNotConfigured
	}
	comment, err := s.Comments.GetIssueComment(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if comment == nil || comment.IssueID != issueID {
		return nil, ErrCommentNotFound
	}
	return comment, nil
}

func validateCommentBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", ErrCommentBodyRequired
	}
	if len(trimmed) > CommentBodyLimit {
		return "", ErrCommentTooLong
	}
	return trimmed, nil
}
