package httpapi

import "net/http"

// Protect applies the common browser-origin and cache policy to API modules.
func Protect(next http.Handler) http.Handler {
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, r, NewProblem(http.StatusForbidden, "forbidden", "Источник запроса не разрешён."))
	}))
	protected := csrf.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		protected.ServeHTTP(w, r)
	})
}
