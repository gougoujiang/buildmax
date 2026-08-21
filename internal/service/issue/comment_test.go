package issue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

func commentService(comments ...model.IssueComment) (*Service, *mock.MockIssueCommentStore) {
	store := &mock.MockIssueCommentStore{Comments: comments}
	return &Service{Issues: &mock.MockIssueStore{}, Comments: store}, store
}

func TestCreateComment(t *testing.T) {
	svc, _ := commentService()
	comment, err := svc.CreateComment(context.Background(), CreateCommentCmd{
		IssueID:    "i_1",
		AuthorKind: model.IssueCommentAuthorUser,
		AuthorID:   "u1",
		Body:       "  blocked on the vendor  ",
	})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.Body != "blocked on the vendor" {
		t.Fatalf("comment.Body = %q, want the trimmed text", comment.Body)
	}
	if comment.EditedAt != nil {
		t.Fatalf("a new comment reports edited_at = %v, want nil", *comment.EditedAt)
	}
}

func TestCreateComment_BodyRequired(t *testing.T) {
	svc, store := commentService()
	_, err := svc.CreateComment(context.Background(), CreateCommentCmd{IssueID: "i_1", AuthorID: "u1", Body: "   \n "})
	if !errors.Is(err, ErrCommentBodyRequired) {
		t.Fatalf("err = %v, want %v", err, ErrCommentBodyRequired)
	}
	if len(store.Comments) != 0 {
		t.Fatalf("whitespace-only body still wrote a row")
	}
}

func TestCreateComment_TooLong(t *testing.T) {
	svc, _ := commentService()
	_, err := svc.CreateComment(context.Background(), CreateCommentCmd{
		IssueID:  "i_1",
		AuthorID: "u1",
		Body:     strings.Repeat("x", CommentBodyLimit+1),
	})
	if !errors.Is(err, ErrCommentTooLong) {
		t.Fatalf("err = %v, want %v", err, ErrCommentTooLong)
	}
}

func TestCreateComment_NotConfigured(t *testing.T) {
	svc := &Service{Issues: &mock.MockIssueStore{}}
	_, err := svc.CreateComment(context.Background(), CreateCommentCmd{IssueID: "i_1", AuthorID: "u1", Body: "hi"})
	if !errors.Is(err, ErrCommentsNotConfigured) {
		t.Fatalf("err = %v, want %v", err, ErrCommentsNotConfigured)
	}
}

func TestUpdateComment_StampsEditedAt(t *testing.T) {
	svc, _ := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorUser, AuthorID: "u1", Body: "first",
	})
	updated, err := svc.UpdateComment(context.Background(), UpdateCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u1", Body: "second",
	})
	if err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if updated.Body != "second" {
		t.Fatalf("updated.Body = %q", updated.Body)
	}
	if updated.EditedAt == nil {
		t.Fatal("updated.EditedAt is nil; an edited comment must say so")
	}
}

func TestUpdateComment_OnlyTheAuthor(t *testing.T) {
	svc, _ := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorUser, AuthorID: "u1", Body: "first",
	})
	_, err := svc.UpdateComment(context.Background(), UpdateCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u2", Body: "rewritten",
	})
	if !errors.Is(err, ErrCommentNotEditable) {
		t.Fatalf("err = %v, want %v", err, ErrCommentNotEditable)
	}
}

// Moderation permits deletion, not rewriting: an agent comment is the record of
// what a run reported.
func TestUpdateComment_AgentCommentIsNotEditable(t *testing.T) {
	svc, _ := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorAgent, AuthorID: "a_1", Body: "run finished",
	})
	_, err := svc.UpdateComment(context.Background(), UpdateCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "a_1", Body: "run failed",
	})
	if !errors.Is(err, ErrCommentNotEditable) {
		t.Fatalf("err = %v, want %v", err, ErrCommentNotEditable)
	}
}

// A comment ID that belongs to another issue is not found, rather than a
// successful write to somewhere the caller was never authorized against.
func TestUpdateComment_WrongIssue(t *testing.T) {
	svc, _ := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_other",
		AuthorKind: model.IssueCommentAuthorUser, AuthorID: "u1", Body: "first",
	})
	_, err := svc.UpdateComment(context.Background(), UpdateCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u1", Body: "second",
	})
	if !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrCommentNotFound)
	}
}

func TestDeleteComment_Author(t *testing.T) {
	svc, store := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorUser, AuthorID: "u1", Body: "first",
	})
	if err := svc.DeleteComment(context.Background(), DeleteCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u1",
	}); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if len(store.Comments) != 0 {
		t.Fatalf("comment survived its author's delete")
	}
}

func TestDeleteComment_StrangerRefused(t *testing.T) {
	svc, store := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorUser, AuthorID: "u1", Body: "first",
	})
	err := svc.DeleteComment(context.Background(), DeleteCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u2",
	})
	if !errors.Is(err, ErrCommentNotEditable) {
		t.Fatalf("err = %v, want %v", err, ErrCommentNotEditable)
	}
	if len(store.Comments) != 1 {
		t.Fatalf("refused delete still removed the comment")
	}
}

func TestDeleteComment_ModeratorMayRemoveAnother(t *testing.T) {
	svc, store := commentService(model.IssueComment{
		IssueCommentID: "ic_1", IssueID: "i_1",
		AuthorKind: model.IssueCommentAuthorAgent, AuthorID: "a_1", Body: "run finished",
	})
	if err := svc.DeleteComment(context.Background(), DeleteCommentCmd{
		IssueID: "i_1", CommentID: "ic_1", UserID: "u_owner", CanModerate: true,
	}); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if len(store.Comments) != 0 {
		t.Fatalf("moderator delete left the comment in place")
	}
}

func TestListComments(t *testing.T) {
	svc, _ := commentService(
		model.IssueComment{IssueCommentID: "ic_1", IssueID: "i_1", Body: "one"},
		model.IssueComment{IssueCommentID: "ic_2", IssueID: "i_other", Body: "two"},
		model.IssueComment{IssueCommentID: "ic_3", IssueID: "i_1", Body: "three"},
	)
	list, total, err := svc.ListComments(context.Background(), "i_1", 50, 0)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("total = %d, len = %d, want 2 and 2", total, len(list))
	}
	if list[0].Body != "one" || list[1].Body != "three" {
		t.Fatalf("thread order = %q, %q; want oldest first", list[0].Body, list[1].Body)
	}
}
