package conversation

import (
	"testing"

	convruntime "buildmax/internal/core/conversation/runtime"
	convtool "buildmax/internal/core/conversation/tool"
)

func TestDefaultConversationTools(t *testing.T) {
	toolList := convruntime.DefaultConversationTools()
	if len(toolList) != 1 {
		t.Fatalf("len(DefaultConversationTools()) = %d, want 1", len(toolList))
	}
	if toolList[0].Name() != convtool.ToolNameGetCurrentDate {
		t.Errorf("toolList[0].Name() = %q, want %s", toolList[0].Name(), convtool.ToolNameGetCurrentDate)
	}
}
