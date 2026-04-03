package tools

import "context"

func ExecuteBatch(ctx context.Context, registry *Registry, execCtx ExecContext, calls []Call) ([]Result, error) {
	results := make([]Result, 0, len(calls))
	for _, call := range calls {
		result, err := registry.Execute(ctx, call, execCtx)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}
