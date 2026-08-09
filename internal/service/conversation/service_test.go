package conversation

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/service/task"
)

func TestTaskServiceForChannel_systemDisablesTaskTools(t *testing.T) {
	ts := &task.TaskService{}
	svc := &ConversationService{TaskService: ts}

	result := svc.taskServiceForChannel(ChannelSystem)

	if result != nil {
		t.Fatalf("system channel task service = %#v, want nil", result)
	}
}

func TestTaskServiceForChannel_portalAllowsTaskTools(t *testing.T) {
	ts := &task.TaskService{}
	svc := &ConversationService{TaskService: ts}

	result := svc.taskServiceForChannel(ChannelPortal)

	if result == nil {
		t.Fatal("portal channel task service = nil, want configured service")
	}
}
