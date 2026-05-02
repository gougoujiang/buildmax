package conversation

import "testing"

func TestDefaultConversationTools(t *testing.T) {
	toolList := DefaultConversationTools()
	if len(toolList) != 1 {
		t.Fatalf("len(DefaultConversationTools()) = %d, want 1", len(toolList))
	}
	if toolList[0].Name() != ToolNameGetCurrentDate {
		t.Errorf("toolList[0].Name() = %q, want %s", toolList[0].Name(), ToolNameGetCurrentDate)
	}
}
