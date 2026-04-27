package tools

func (r *Registry) registerDefaultTools() {
	r.registerPlanningTools()
	r.registerFileTools()
	r.registerAutomationTools()
	r.registerRoleTools()
	r.registerTaskTools()
	r.registerRuntimeTools()
	r.registerWorkspaceTools()
}

func (r *Registry) registerPlanningTools() {
	r.Register(enterPlanModeTool{})
	r.Register(exitPlanModeTool{})
	r.Register(todoWriteTool{})
}

func (r *Registry) registerFileTools() {
	r.Register(fileReadTool{})
	r.Register(fileWriteTool{})
	r.Register(fileEditTool{})
	r.Register(applyPatchTool{})
	r.Register(globTool{})
	r.Register(grepTool{})
	r.Register(listDirTool{})
	r.Register(notebookEditTool{})
}

func (r *Registry) registerAutomationTools() {
	r.Register(automationCreateSimpleTool{})
	r.Register(automationControlTool{})
	r.Register(automationGetTool{})
	r.Register(automationListTool{})
	r.Register(automationQueryTool{})
	r.Register(scheduleReportTool{})
	r.Register(scheduleQueryTool{})
}

func (r *Registry) registerRoleTools() {
	r.Register(delegateToRoleTool{})
	r.Register(roleCreateTool{})
	r.Register(roleGetTool{})
	r.Register(roleListTool{})
	r.Register(roleUpdateTool{})
}

func (r *Registry) registerTaskTools() {
	r.Register(taskCreateTool{})
	r.Register(taskGetTool{})
	r.Register(taskListTool{})
	r.Register(taskOutputTool{})
	r.Register(taskResultTool{})
	r.Register(taskWaitTool{})
	r.Register(taskStopTool{})
	r.Register(taskUpdateTool{})
}

func (r *Registry) registerRuntimeTools() {
	r.Register(analyzeImageTool{})
	r.Register(requestUserInputTool{})
	r.Register(shellTool{})
	r.Register(skillUseTool{})
	r.Register(viewImageTool{})
	r.Register(webFetchTool{})
}

func (r *Registry) registerWorkspaceTools() {
	r.Register(enterWorktreeTool{})
	r.Register(exitWorktreeTool{})
}
