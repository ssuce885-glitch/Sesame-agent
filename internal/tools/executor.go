package tools

import "context"

func ExecuteBatch(ctx context.Context, registry *Registry, execCtx ExecContext, calls []Call) ([]Result, error) {
	executed, err := NewRuntime(registry, nil).ExecuteCalls(ctx, calls, execCtx)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(executed))
	for _, item := range executed {
		if item.Err != nil {
			return nil, item.Err
		}
		results = append(results, item.Result)
	}
	return results, nil
}
