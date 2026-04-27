package tools

import (
	"sort"
	"strings"
)

type Registry struct {
	tools       map[string]Tool
	aliases     map[string]string
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	r := &Registry{
		tools:       make(map[string]Tool),
		aliases:     make(map[string]string),
		definitions: make(map[string]Definition),
	}
	r.registerDefaultTools()
	return r
}

func (r *Registry) Register(tool Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	if r.definitions == nil {
		r.definitions = make(map[string]Definition)
	}

	def := tool.Definition()
	r.tools[def.Name] = tool
	r.definitions[def.Name] = cloneDefinition(def)
	for _, alias := range def.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == def.Name {
			continue
		}
		r.aliases[alias] = def.Name
	}
}

func (r *Registry) Definitions() []Definition {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, cloneDefinition(r.definitions[name]))
	}
	return defs
}

func (r *Registry) VisibleDefinitions(execCtx ExecContext) []Definition {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		if !toolEnabled(tool, execCtx) {
			continue
		}
		defs = append(defs, cloneDefinition(r.definitions[name]))
	}
	return defs
}

func (r *Registry) lookup(name string) (Tool, Definition, string, bool) {
	if r == nil {
		return nil, Definition{}, "", false
	}
	if tool, ok := r.tools[name]; ok {
		def, ok := r.definitions[name]
		if !ok {
			return nil, Definition{}, "", false
		}
		return tool, cloneDefinition(def), name, true
	}
	canonical, ok := r.aliases[name]
	if !ok {
		return nil, Definition{}, "", false
	}
	tool, ok := r.tools[canonical]
	if !ok {
		return nil, Definition{}, "", false
	}
	def, ok := r.definitions[canonical]
	if !ok {
		return nil, Definition{}, "", false
	}
	return tool, cloneDefinition(def), canonical, true
}
