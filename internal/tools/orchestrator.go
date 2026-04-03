package tools

func PartitionByConcurrency(calls []Call, registry *Registry) (parallel []Call, serial []Call) {
	for _, call := range calls {
		tool := registry.tools[call.Name]
		if tool != nil && tool.IsConcurrencySafe() {
			parallel = append(parallel, call)
			continue
		}
		serial = append(serial, call)
	}

	return parallel, serial
}
