package tools

import (
	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/contracts"
)

func RegisterAllTools(r contracts.ToolRegistry, roleService RoleService, catalog skillcatalog.Catalog) {
	r.Register(contracts.NamespaceShell, NewShellTool())
	RegisterFilesTools(r)
	r.Register(contracts.NamespaceWorkspace, NewToolPolicyExplainTool(r))
	r.Register(contracts.NamespaceMemory, NewMemoryWriteTool())
	r.Register(contracts.NamespaceMemory, NewRecallArchiveTool())
	r.Register(contracts.NamespaceMemory, NewLoadContextTool())
	r.Register(contracts.NamespaceRoles, NewRoleListTool(roleService))
	r.Register(contracts.NamespaceRoles, NewRoleCreateTool(roleService))
	r.Register(contracts.NamespaceRoles, NewRoleUpdateTool(roleService))
	r.Register(contracts.NamespaceRoles, NewRoleInstallTool(roleService))
	r.Register(contracts.NamespaceTasks, NewTaskTraceTool())
	r.Register(contracts.NamespaceSkill, NewSkillUseTool(catalog))
	r.Register(contracts.NamespaceAutomation, NewAutomationCreateSimpleTool())
	r.Register(contracts.NamespaceAutomation, NewAutomationQueryTool())
	r.Register(contracts.NamespaceAutomation, NewAutomationControlTool())
}
