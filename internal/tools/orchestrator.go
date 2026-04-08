package tools

func PartitionByConcurrency(calls []Call, registry *Registry) (parallel []Call, serial []Call) {
	for _, batch := range NewRuntime(registry, nil).PlanBatches(calls, ExecContext{}) {
		for _, prepared := range batch.Calls {
			if batch.Parallel {
				parallel = append(parallel, prepared.Original)
				continue
			}
			serial = append(serial, prepared.Original)
		}
	}

	return parallel, serial
}
