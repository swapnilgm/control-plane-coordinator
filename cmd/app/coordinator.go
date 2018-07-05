// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/gardener/control-plane-coordinator/pkg/componentconfig"
	"github.com/gardener/control-plane-coordinator/pkg/server"
	"github.com/gardener/control-plane-coordinator/pkg/server/handler"
	"github.com/gardener/control-plane-coordinator/pkg/version"
	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Options has all the context and parameters needed to run a Shoot Control plane coordinator.
type Options struct {
	// ConfigFile is the location of the Control plane coordinator's configuration file.
	ConfigFile string
	config     *componentconfig.CoordinatorConfiguration
}

// addFlags adds flags for a specific Control plane coordinator to the specified FlagSet.
func addFlags(options *Options, fs *pflag.FlagSet) {
	fs.StringVar(&options.ConfigFile, "config", options.ConfigFile, "The path to the configuration file.")
}

// newOptions returns a new Options object.
func newOptions() (*Options, error) {
	o := &Options{
		config: new(componentconfig.CoordinatorConfiguration),
	}
	return o, nil
}

// loadConfigFromFile loads the contents of file and decodes it as a
// ControllerManagerConfiguration object.
func (o *Options) loadConfigFromFile(file string) (*componentconfig.CoordinatorConfiguration, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return o.decodeConfig(data)
}

// decodeConfig decodes data as a CoordinatorConfiguration object.
func (o *Options) decodeConfig(data []byte) (*componentconfig.CoordinatorConfiguration, error) {
	var config componentconfig.CoordinatorConfiguration
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (o *Options) configFileSpecified() error {
	if len(o.ConfigFile) == 0 {
		return fmt.Errorf("missing Control plane coordinator config file")
	}
	return nil
}

// Validate validates all the required options.
func (o *Options) validate(args []string) error {
	if len(args) != 0 {
		return errors.New("arguments are not supported")
	}
	return nil
}

func (o *Options) applyDefaults(in *componentconfig.CoordinatorConfiguration) (*componentconfig.CoordinatorConfiguration, error) {
	//TODO
	return in, nil
}

func (o *Options) run(stopCh <-chan struct{}) error {

	if len(o.ConfigFile) > 0 {
		c, err := o.loadConfigFromFile(o.ConfigFile)
		if err != nil {
			return err
		}
		o.config = c
	}

	coord, err := newCoordinator(o.config)
	if err != nil {
		return err
	}

	return coord.Run(stopCh)
}

// NewCommandStartCoordinator creates a *cobra.Command object with default parameters
func NewCommandStartCoordinator(stopCh <-chan struct{}) *cobra.Command {
	opts, err := newOptions()
	if err != nil {
		panic(err)
	}

	cmd := &cobra.Command{
		Use:   "control-plane-coordinator",
		Short: "Launch the Control plane coordinator",
		Long: `In essence, the Control plane coordinator is an centralized coordinator for control
plane components of shoot which will be hosted/migrated across different seed. The coordinator 
based on DNS record for shoot APIServer determines the associated control plane should be active or
passive.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opts.configFileSpecified(); err != nil {
				panic(err)
			}
			if err := opts.validate(args); err != nil {
				panic(err)
			}
			if err := opts.run(stopCh); err != nil {
				panic(err)
			}
		},
	}

	opts.config, err = opts.applyDefaults(opts.config)
	if err != nil {
		panic(err)
	}
	addFlags(opts, cmd.Flags())
	return cmd
}

// Coordinator represents all the parameters required to start the coordinator.
type Coordinator struct {
	Config *componentconfig.CoordinatorConfiguration
	Logger *logrus.Logger
}

// newCoordinator is the main entry point of instantiating a new Gardener controller manager.
func newCoordinator(config *componentconfig.CoordinatorConfiguration) (*Coordinator, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	applyEnvironmentToConfig(config)

	// Initialize logger
	logger := logrus.New()
	logger.SetLevel(logrus.Level(config.LogLevel))
	logger.Infof("Control plane coordinator version: %s", version.Version)
	logger.Info("Starting Control plane coordinator...")

	return &Coordinator{
		Config: config,
		Logger: logger,
	}, nil
}

// Run runs the Coordinator. This should never exit.
func (c *Coordinator) Run(stopCh <-chan struct{}) error {
	// Start HTTP server
	go server.Serve(c.Config.Server.BindAddress, c.Config.Server.Port)
	handler.UpdateHealth(true)

	select {
	case <-stopCh:
		c.Logger.Infof("Received stop signal.\nTerminating... \nGood bye!")
		return nil
	}
}

// applyEnvironmentToConfig checks for several well-defined environment variables and if they are set,
// it sets the value of the respective keys of <config> to the values in the environment.
// Currently unimplemented environment variables:
// KUBECONFIG can override config.ClientConnection.KubeConfigFile
func applyEnvironmentToConfig(config *componentconfig.CoordinatorConfiguration) {
	//TODO
	/*if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config.ClientConnection.KubeConfigFile = kubeconfig
	}*/
}
