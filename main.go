package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/ogier/pflag"
)

func main() {
	var configFile = pflag.StringP("config", "c", "log-pulse.yml", "The yaml file to load configuration from")
	pflag.Parse()

	config, err := ParseConfigFile(*configFile)
	if err != nil {
		log.Fatal("Unable to parse file: ", err)
	}

	monitor := CreateMonitor(config)

	sigs := make(chan os.Signal, 1)
	go func() {
		s := <-sigs
		log.Println("Received exit signal: ", s)
		log.Println("Stopping and cleaning up")
		monitor.Stop()
	}()

	signal.Notify(sigs, os.Interrupt, os.Kill)

	monitor.Start()
}
