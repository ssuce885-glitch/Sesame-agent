package httpapi

import "net/http"

type Dependencies struct {
	Bus     any
	Store   any
	Manager any
}

func NewTestDependencies(t interface{ Helper() }) Dependencies {
	t.Helper()
	return Dependencies{}
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	registerStatusRoutes(mux)
	registerSessionRoutes(mux, deps)
	registerMemoryRoutes(mux, deps)

	return mux
}
