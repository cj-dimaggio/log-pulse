package main

import (
	"os"
	"os/signal"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/ogier/pflag"
)

func main() {
	configFile := pflag.StringP("config", "c", "log-pulse.yml", "The yaml file to load configuration from")
	logLevel := pflag.String("loglevel", "INFO", "The lowest log level you want outputted")

	pflag.Parse()

	// Initialize our logging
	logp.Init("log-pulse", &logp.Logging{
		Level: *logLevel,
	})

	// Load our configuration
	configs, rawConfigs, err := ParseConfigFile(*configFile)
	if err != nil {
		logp.Critical("Unable to parse the config file: %s", err)
		os.Exit(1)
	}

	// Create our Collection
	collection, err := CreateCollection(*configs, rawConfigs)
	if err != nil {
		logp.Critical("Unable to create a collection: %s", err)
		os.Exit(1)
	}

	// Register with exit signals
	sigs := make(chan os.Signal, 1)
	go func() {
		s := <-sigs
		logp.Info("Received exit signal: %s", s)
		logp.Info("Stopping and cleaning up")
		collection.Stop()
	}()
	signal.Notify(sigs, os.Interrupt, os.Kill)

	// Start our process
	collection.Start()
	collection.LetRun()
}
