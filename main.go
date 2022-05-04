package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/suborbital/vektor/vlog"
	"github.com/suborbital/velocity/server"
	"github.com/suborbital/velocity/server/options"
	"github.com/suborbital/velocity/server/release"
)

const (
	headlessFlag = "headless"
	waitFlag     = "wait"
	appNameFlag  = "appName"
	domainFlag   = "domain"
	httpPortFlag = "httpPort"
	tlsPortFlag  = "tlsPort"
)

type atmoInfo struct {
	AtmoVersion string `json:"atmo_version"`
}

func main() {
	cmd := rootCommand()
	cmd.Execute()
}

func rootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "velocity [bundle-path]",
		Short: "Velocity cloud native function framework",
		Long: `
Velocity is an all-in-one cloud native functions framework that enables 
building backend systems using composable WebAssembly modules in a declarative manner.

Velocity automatically extends any application with stateless, ephemeral functions that
execute within a secure sandbox, written in any language. 

Handling API and event-based traffic is made simple using the declarative 
Directive format and the powerful API available for many languages.`,
		Version: release.VelocityServerDotVersion,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "./runnables.wasm.zip"
			if len(args) > 0 {
				path = args[0]
			}

			logger := vlog.Default(
				vlog.AppMeta(atmoInfo{AtmoVersion: release.VelocityServerDotVersion}),
				vlog.Level(vlog.LogLevelInfo),
				vlog.EnvPrefix("ATMO"),
			)

			opts, err := optionsFromFlags(cmd.Flags())
			if err != nil {
				return errors.Wrap(err, "failed to optionsFromFlags")
			}

			atmoService, err := server.New(
				append(
					opts,
					options.UseLogger(logger),
					options.UseBundlePath(path),
				)...,
			)
			if err != nil {
				return errors.Wrap(err, "atmo.New")
			}

			return atmoService.Start()
		},
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	cmd.Flags().Bool(waitFlag, false, "if passed, Atmo will wait until a bundle becomes available on disk, checking once per second")
	cmd.Flags().String(appNameFlag, "Atmo", "if passed, it'll be used as ATMO_APP_NAME, otherwise 'Atmo' will be used")
	cmd.Flags().String(domainFlag, "", "if passed, it'll be used as ATMO_DOMAIN and HTTPS will be used, otherwise HTTP will be used")
	cmd.Flags().Int(httpPortFlag, 8080, "if passed, it'll be used as ATMO_HTTP_PORT, otherwise '8080' will be used")
	cmd.Flags().Int(tlsPortFlag, 443, "if passed, it'll be used as ATMO_TLS_PORT, otherwise '443' will be used")

	return cmd
}

func optionsFromFlags(flags *pflag.FlagSet) ([]options.Modifier, error) {
	appName, err := flags.GetString(appNameFlag)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("get string flag '%s' value", appNameFlag))
	}

	domain, err := flags.GetString(domainFlag)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("get string flag '%s' value", domainFlag))
	}

	httpPort, err := flags.GetInt(httpPortFlag)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("get int flag '%s' value", httpPortFlag))
	}

	tlsPort, err := flags.GetInt(tlsPortFlag)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("get int flag '%s' value", tlsPortFlag))
	}

	shouldWait := flags.Changed(waitFlag)
	shouldRunHeadless := flags.Changed(headlessFlag)

	opts := []options.Modifier{
		options.ShouldRunHeadless(shouldRunHeadless),
		options.ShouldWait(shouldWait),
		options.AppName(appName),
		options.Domain(domain),
		options.HTTPPort(httpPort),
		options.TLSPort(tlsPort),
	}

	return opts, nil
}
