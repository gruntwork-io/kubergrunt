package main

import (
	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/gruntwork-cli/errors"
	"github.com/gruntwork-io/gruntwork-cli/logging"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

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

	app.Before = initCli

	app.Flags = []cli.Flag{
		logLevelFlag,
	}
	app.Commands = []cli.Command{
		SetupEksCommand(),
	}
	entrypoint.RunApp(app)
}
