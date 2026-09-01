package main

import (
	"azugo.io/azugo/server"
	"azugo.io/core/cli"

	app "github.com/go-make-bytes/eidas-audit"
)

func init() {
	cli.Register(server.HealthCommand("/healthz", server.Options{
		AppName:       "eIDAS Audit & Evidence Service",
		AppVer:        Version,
		Configuration: app.NewConfiguration(),
	}))
}
