package httpapi

import "net/http"

type Dependencies struct{}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	registerStatusRoutes(mux)
	registerSessionRoutes(mux, deps)
	registerTurnRoutes(mux, deps)
	registerEventRoutes(mux, deps)

	return mux
}
