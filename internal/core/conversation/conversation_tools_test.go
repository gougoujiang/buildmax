package conversation

import (
	"testing"

	convtool "buildmax/internal/core/conversation/tool"
)

func TestDefaultConversationTools(t *testing.T) {
	toolList := DefaultConversationTools()
	if len(toolList) != 1 {
		t.Fatalf("len(DefaultConversationTools()) = %d, want 1", len(toolList))
	}
	if toolList[0].Name() != convtool.ToolNameGetCurrentDate {
		t.Errorf("toolList[0].Name() = %q, want %s", toolList[0].Name(), convtool.ToolNameGetCurrentDate)
	}
}
