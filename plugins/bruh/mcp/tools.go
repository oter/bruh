package main

// AllTools lists every tool of the server. Tasks 4 to 7 add their tools here.
func AllTools() []Tool {
	var tools []Tool
	tools = append(tools, mailTools()...)
	tools = append(tools, handoffTools()...)
	return tools
}
