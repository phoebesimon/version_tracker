package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/phoebesimon/version_tracker/tracker"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var Version = "0.0.0"

func main() {
	app := cli.NewApp()
	app.Name = "latest-os-version-tracker"
	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "interval",
			Usage: "How often (in seconds) to check if a new patch is out (defaults to 300)",
			Value: 300,
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enables debug-level logging",
		},
	}

	app.Action = func(c *cli.Context) error {
		interval := c.Int("interval")

		if c.IsSet("debug") {
			log.SetLevel(log.DebugLevel)
		}

		done := make(chan os.Signal, 1)

		signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

		ctx, cancel := context.WithCancel(context.Background())

		versionTracker := tracker.MakeTracker(interval)
		go versionTracker.Start(ctx)

		<-done
		cancel()

		versionTracker.Close()

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("error running burrow-exporter")
		os.Exit(1)
	}

}
