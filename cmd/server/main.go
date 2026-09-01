package main

import (
	"os"

	"azugo.io/core/cli"
)

// Version is set at build time.
var Version = "0.1.0-dev"

func main() {
	if _, ok := os.LookupEnv("SERVER_URLS"); !ok {
		_ = os.Setenv("SERVER_URLS", "http://0.0.0.0:8080")
	}

	cli.Run(cli.Options{
		Use:     "eidas-audit",
		Short:   "eIDAS Audit & Evidence Service",
		Long:    "eIDAS-audit (eIDAS/ETSI signing evidence) audit sink: consumes the audit.signing broker stream into the hash-chained eidas_audit store. Starts the web server (+ broker consumer) by default.",
		Version: Version,
	})
}
