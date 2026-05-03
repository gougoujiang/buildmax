package tool

import "buildmax/internal/core/model"

const (
	ToolNameGetCurrentDate = "GetCurrentDate"
	ToolNameStartTask      = "StartTask"
	ToolNameListTasks      = "ListTasks"
	ToolNameGetTask        = "GetTask"
	ToolNameContinueTask   = "ContinueTask"
)

// DefaultTools returns the default Tier 1 conversation tool set.
func DefaultTools() []model.Tool {
	return []model.Tool{GetCurrentDate{}}
}
