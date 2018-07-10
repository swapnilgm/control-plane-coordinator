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
	"net"
	"os"
	"time"

	"github.com/gardener/control-plane-coordinator/pkg/componentconfig"
	controlplanecontroller "github.com/gardener/control-plane-coordinator/pkg/controller/controlplane"
	"github.com/gardener/control-plane-coordinator/pkg/dns"
	"github.com/gardener/control-plane-coordinator/pkg/server"
	"github.com/gardener/control-plane-coordinator/pkg/server/handler"
	"github.com/gardener/control-plane-coordinator/pkg/version"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
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

// applyDefaults applies the default values to component configurations
func (o *Options) applyDefaults(in *componentconfig.CoordinatorConfiguration) (*componentconfig.CoordinatorConfiguration, error) {
	componentconfig.SetDefaultsCoordinatorConfiguration(in)
	return in, nil
}

func (o *Options) run(stopCh <-chan struct{}) error {
	if len(o.ConfigFile) > 0 {
		config, err := o.loadConfigFromFile(o.ConfigFile)
		if err != nil {
			return err
		}
		o.config = config
	}

	c, err := newCoordinator(o.config)
	if err != nil {
		return err
	}

	return c.Run(stopCh)
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
	Config    *componentconfig.CoordinatorConfiguration
	Logger    *logrus.Logger
	K8sClient kubernetes.Client
	Recorder  record.EventRecorder
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

	// Prepare a Kubernetes client object for the hosted seed cluster which contains all the Clientsets
	// that can be used to access the Kubernetes API.
	logrus.Infof("kubeconfig: %v", config.KubeconfigFile)
	k8sClient, err := kubernetes.NewClientFromFile(config.KubeconfigFile)
	if err != nil {
		return nil, err
	}

	return &Coordinator{
		Config:    config,
		Logger:    logger,
		K8sClient: k8sClient,
		Recorder:  createRecorder(k8sClient.Clientset()),
	}, nil
}

// Run runs the Coordinator. This should never exit.
func (c *Coordinator) Run(stopCh <-chan struct{}) error {
	lookupCh := make(chan bool)
	// Start HTTP server
	go server.Serve(c.Config.Server.BindAddress, c.Config.Server.Port)
	handler.UpdateHealth(false)
	go startControllers(c, stopCh, lookupCh)
	for {
		c.Logger.Infof("In for loop")
		select {
		case <-stopCh:
			c.Logger.Infof("Received stop signal.\nTerminating...\nGood bye!")
			return nil
		case <-time.After(10 * time.Second):
			c.lookupDNS()
		case <-lookupCh:
			c.lookupDNS()

		}
	}
}

func (c *Coordinator) lookupDNS() {
	c.Logger.Infof("Received lookup signal")
	svc, err := c.K8sClient.Clientset().Core().Services(c.Config.WatchNamespace).Get(c.Config.Service, metav1.GetOptions{})
	if err != nil {
		c.Logger.Errorf("failed to get service: %v", err)
		handler.UpdateHealth(false)
		return
	}
	valid := false
	for _, lb := range svc.Status.LoadBalancer.Ingress {
		if len(lb.IP) >= 0 {
			valid, err = dns.ValidateDNSWithIP(c.Config.LookupEndpoint, net.ParseIP(c.Config.ExpectedIP))
			if err != nil {
				c.Logger.Errorf("failed to validate the DNS resolution: %v", err)
				valid = false
			}
		} else if len(lb.Hostname) > 0 {
			valid, err = dns.ValidateDNSWithCname(c.Config.LookupEndpoint, lb.Hostname)
			if err != nil {
				c.Logger.Errorf("failed to validate the DNS resolution: %v", err)
				valid = false
			}
		}
	}
	if valid {
		c.Logger.Infof("Lookup DNS %s resolved to expected value. Nothing to be done.", c.Config.LookupEndpoint)
		handler.UpdateHealth(true)
	} else {
		c.Logger.Infof("Lookup DNS %s didn't resolved to expected value %s.", c.Config.LookupEndpoint)
		handler.UpdateHealth(false)
	}
}

func startControllers(c *Coordinator, stopCh <-chan struct{}, lookupCh chan<- bool) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(c.K8sClient.Clientset(), 30*time.Second, kubeinformers.WithNamespace(c.Config.WatchNamespace))
	servicesInformer := kubeInformerFactory.Core().V1().Services().Informer()
	kubeInformerFactory.Start(stopCh)
	if !cache.WaitForCacheSync(make(<-chan struct{}), servicesInformer.HasSynced) {
		panic("Timed out waiting for caches to sync")
	}
	controlplaneController := controlplanecontroller.NewControlplaneController(c.K8sClient, kubeInformerFactory, c.Config, c.Recorder, lookupCh)
	concurrentSyncs := 5
	go controlplaneController.Run(concurrentSyncs, stopCh)
	logrus.Infof("Controlplane coordinator (version %s) initialized.", version.Version)

	// Shutdown handling
	<-stopCh
	logrus.Info("I have received a stop signal and will no longer watch events of the kubernetes API group.")
	logrus.Infof("Number of remaining workers -- Controlplane: %d", controlplaneController.RunningWorkers())
	logrus.Info("Bye bye!")
}

func createRecorder(kubeClient *k8s.Clientset) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Debugf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: typedcorev1.New(kubeClient.CoreV1().RESTClient()).Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "control-plane-coordinator"})
}

// applyEnvironmentToConfig checks for several well-defined environment variables and if they are set,
// it sets the value of the respective keys of <config> to the values in the environment.
// Currently implemented environment variables:
// KUBECONFIG can override config.KubeconfigFile
func applyEnvironmentToConfig(config *componentconfig.CoordinatorConfiguration) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config.KubeconfigFile = kubeconfig
	}
}
