package metrics

import (
	"crypto/subtle"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// BasicAuthFilterProvider returns a FilterProvider that validates HTTP Basic
// Auth credentials on the metrics endpoint. The returned function matches
// the signature expected by metricsserver.Options.FilterProvider.
//
// The *rest.Config and *http.Client parameters are required by the
// FilterProvider interface but unused here (they are used by the
// Kubernetes RBAC-based filter, not by basicAuth).
func BasicAuthFilterProvider(username, password string) func(*rest.Config, *http.Client) (metricsserver.Filter, error) {
	return func(_ *rest.Config, _ *http.Client) (metricsserver.Filter, error) {
		return func(log logr.Logger, handler http.Handler) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u, p, ok := r.BasicAuth()
				userMatch := subtle.ConstantTimeCompare([]byte(u), []byte(username)) == 1
				passMatch := subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1
				if !ok || !userMatch || !passMatch {
					log.Info("Metrics auth failed", "remote", r.RemoteAddr)
					w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				handler.ServeHTTP(w, r)
			}), nil
		}, nil
	}
}
