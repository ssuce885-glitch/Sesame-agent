package contextstate

func WithinBudget(current, limit int) bool {
	return current <= limit
}
