package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/phoebesimon/version_tracker/tracker"
	"github.com/urfave/cli"
)

var Version = "0.0.0"

func main() {
	app := cli.NewApp()
	app.Name = "latest-os-version-tracker"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "addr",
			Usage: "The address the tracker is listening on (i.e. localhost:2370)",
		},
		cli.StringFlag{
			Name:  "interval",
			Usage: "How often (in seconds) to check if a new patch is out (defaults to 60)",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enables debug-level logging",
		},
	}

	app.Action = func(c *cli.Context) error {
		interval := 20
		if c.IsSet("interval") {
			interval = c.Int("interval")
		}

		//if !c.IsSet("addr") {
		//	fmt.Println("An address to run prometheus on is required (e.g. --metrics-addr localhost:8080)")
		//	os.Exit(1)
		//}

		if c.IsSet("debug") {
			log.SetLevel(log.DebugLevel)
		}

		done := make(chan os.Signal, 1)

		signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

		ctx, cancel := context.WithCancel(context.Background())

		exporter := tracker.MakeTracker(c.String("addr"), interval)
		go exporter.Start(ctx)

		<-done
		cancel()

		exporter.Close()

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
