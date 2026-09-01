// Package routes registers the eidas-audit HTTP surface. For the MVP that is only
// the unauthenticated liveness/readiness probes — the service's real work is the
// background broker consumer (no inbound API). The future DSAR/verify/chain-head
// read API will be added here behind go-authbyte (audience svc:eidas-audit).
package routes

import (
	app "github.com/go-make-bytes/eidas-audit"
)

// router binds handlers to the application.
type router struct {
	*app.App
}

// Init registers the routes on the application.
func Init(a *app.App) error {
	r := &router{App: a}

	a.Get("/healthz", r.healthz)
	a.Get("/readyz", r.readyz)

	return nil
}
