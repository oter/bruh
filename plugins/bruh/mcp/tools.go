package main

// AllTools lists every tool of the server. Tasks 4 to 7 add their tools here.
func AllTools() []Tool {
	var tools []Tool
	tools = append(tools, mailTools()...)
	tools = append(tools, handoffTools()...)
	tools = append(tools, answerTools()...)
	tools = append(tools, reportTools()...)
	tools = append(tools, rolesTools()...)
	tools = append(tools, leaseTools()...)
	tools = append(tools, questionTools()...)
	tools = append(tools, sessionTools()...)
	tools = append(tools, initTools()...)
	tools = append(tools, reposTools()...)
	tools = append(tools, resultTools()...)
	tools = append(tools, learnRefreshTool())
	tools = append(tools, learnScanTool())
	tools = append(tools, monitorTools()...)
	return tools
}
