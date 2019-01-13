package main

import (
	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/logging"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// This variable is set at build time using -ldflags parameters. For example, we typically set this flag in circle.yml
// to the latest Git tag when building our Go apps:
//
// build-go-binaries --app-name my-app --dest-path bin --ld-flags "-X main.VERSION=$CIRCLE_TAG"
//
// For more info, see: http://stackoverflow.com/a/11355611/483528
var VERSION string

var (
	logLevelFlag = cli.StringFlag{
		Name:  "loglevel",
		Value: logrus.InfoLevel.String(),
	}
)

// initCli initializes the CLI app before any command is actually executed. This function will handle all the setup
// code, such as setting up the logger with the appropriate log level.
func initCli(cliContext *cli.Context) error {
	// Set logging level
	logLevel := cliContext.String(logLevelFlag.Name)
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	logging.SetGlobalLogLevel(level)
	return nil
}

// main should only setup the CLI flags and help texts.
func main() {
	app := entrypoint.NewApp()
	entrypoint.HelpTextLineWidth = 120

	app.Name = "kubergrunt"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Description = "A CLI tool to help setup and manage a Kubernetes cluster."
	app.EnableBashCompletion = true
	// Set the version number from your app from the VERSION variable that is passed in at build time
	app.Version = VERSION

	app.Before = initCli

	app.Flags = []cli.Flag{
		logLevelFlag,
	}
	app.Commands = []cli.Command{
		SetupEksCommand(),
		SetupHelmCommand(),
	}
	entrypoint.RunApp(app)
}
