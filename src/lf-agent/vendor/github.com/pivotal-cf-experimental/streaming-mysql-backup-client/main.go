package main

import (
	"log"
	"os"

	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/client"
	"github.com/pivotal-cf-experimental/streaming-mysql-backup-client/config"
)

func main() {

	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger

	if err != nil {
		logger.Fatal("Error parsing config file", err)
	}

	client := client.DefaultClient(*rootConfig)
	if err := client.Execute(); err != nil {
		log.Fatalf("All backups failed. Not able to generate a valid backup artifact. See error(s) below: %s", err)
	}
}
