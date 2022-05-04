package orchestrator

import (
	"log"
	"runtime"
	"time"

	"github.com/pkg/errors"

	"github.com/suborbital/vektor/vlog"
	"github.com/suborbital/velocity/orchestrator/exec"
	"github.com/suborbital/velocity/server/appsource"

	"github.com/suborbital/velocity/orchestrator/config"
)

const (
	atmoPort = "8080"
)

type Orchestrator struct {
	logger *vlog.Logger
	config config.Config
	sats   map[string]*watcher // map of FQFNs to watchers
}

func New(bundlePath string) (*Orchestrator, error) {
	conf, err := config.Parse(bundlePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to config.Parse")
	}

	l := vlog.Default(
		vlog.EnvPrefix("APPSOURCE"),
	)

	o := &Orchestrator{
		logger: l,
		config: conf,
		sats:   map[string]*watcher{},
	}

	return o, nil
}

func (o *Orchestrator) Start() {
	appSource, errChan := o.setupAppSource()

	// main event loop
	go func() {
		for {
			o.reconcileConstellation(appSource, errChan)

			time.Sleep(time.Second)
		}
	}()

	// assuming nothing above throws an error, this will block forever
	for err := range errChan {
		log.Fatal(errors.Wrap(err, "encountered error"))
	}
}

func (o *Orchestrator) reconcileConstellation(appSource appsource.AppSource, errChan chan error) {
	apps := appSource.Applications()

	for _, app := range apps {
		runnables := appSource.Runnables(app.Identifier, app.AppVersion)

		for i := range runnables {
			runnable := runnables[i]

			o.logger.Debug("reconciling", runnable.FQFN)

			if _, exists := o.sats[runnable.FQFN]; !exists {
				o.sats[runnable.FQFN] = newWatcher(runnable.FQFN, o.logger)
			}

			satWatcher := o.sats[runnable.FQFN]

			launch := func() {
				cmd, port := satCommand(o.config, runnable)

				// repeat forever in case the command does error out
				uuid, pid, err := exec.Run(
					cmd,
					"SAT_HTTP_PORT="+port,
					"SAT_ENV_TOKEN="+o.config.EnvToken,
					"SAT_CONTROL_PLANE="+o.config.ControlPlane,
				)

				if err != nil {
					errChan <- errors.Wrap(err, "sat exited with error")
				}

				satWatcher.add(port, uuid, pid)
			}

			// we want to max out at 8 threads per instance
			threshold := runtime.NumCPU() / 2
			if threshold > 8 {
				threshold = 8
			}

			report := satWatcher.report()
			if report == nil {
				// if no instances exist, launch one
				o.logger.Warn("launching", runnable.FQFN)

				go launch()
			} else if report.instCount > 0 && report.totalThreads/report.instCount >= threshold {
				if report.instCount >= runtime.NumCPU() {
					o.logger.Warn("maximum instance count reached for", runnable.Name)
				} else {
					// if the current instances seem overwhelmed, add one
					o.logger.Warn("scaling up", runnable.Name, "; totalThreads:", report.totalThreads, "instCount:", report.instCount)

					go launch()
				}
			} else if report.instCount > 0 && report.totalThreads/report.instCount < threshold {
				if report.instCount == 1 {
					// that's fine, do nothing
				} else {
					// if the current instances have too much spare time on their hands
					o.logger.Warn("scaling down", runnable.Name, "; totalThreads:", report.totalThreads, "instCount:", report.instCount)

					satWatcher.terminate()
				}
			}

			if report != nil {
				for _, p := range report.failedPorts {
					o.logger.Warn("killing instance from failed port", p)

					satWatcher.terminateInstance(p)
				}
			}
		}
	}
}

func (o *Orchestrator) setupAppSource() (appsource.AppSource, chan error) {
	// if an external control plane hasn't been set, act as the control plane
	// but if one has been set, use it (and launch all children with it configured)
	if o.config.ControlPlane == config.DefaultControlPlane {
		appSource, errChan := startAppSourceServer(o.config.BundlePath)
		return appSource, errChan
	}

	appSource := appsource.NewHTTPSource(o.config.ControlPlane)

	if err := startAppSourceWithRetry(o.logger, appSource); err != nil {
		log.Fatal(errors.Wrap(err, "failed to startAppSourceHTTPClient"))
	}

	if err := registerWithControlPlane(o.config); err != nil {
		log.Fatal(errors.Wrap(err, "failed to registerWithControlPlane"))
	}

	errChan := make(chan error)

	return appSource, errChan
}
