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

package controlplane

import (
	"sync"
	"time"

	"github.com/gardener/control-plane-coordinator/pkg/componentconfig"
	"github.com/gardener/control-plane-coordinator/pkg/controller/utils"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/sirupsen/logrus"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

// Controller controls Shoots.
type Controller struct {
	k8sClient    kubernetes.Client
	k8sInformers kubeinformers.SharedInformerFactory

	config   *componentconfig.CoordinatorConfiguration
	control  ControlInterface
	recorder record.EventRecorder

	serviceLister v1.ServiceLister
	serviceQueue  workqueue.RateLimitingInterface

	serviceSynced cache.InformerSynced

	numberOfRunningWorkers int
	workerCh               chan int
}

// NewControlplaneController takes a Kubernetes client for the Seed clusters <k8sClient>, a <k8sInformer>,
// and a <recorder> for event recording. It creates a new Controlplane controller.
func NewControlplaneController(k8sClient kubernetes.Client, k8sInformers kubeinformers.SharedInformerFactory, config *componentconfig.CoordinatorConfiguration, recorder record.EventRecorder, lookupCh chan<- bool) *Controller {
	var (
		k8sInformer     = k8sInformers.Core().V1()
		serviceInformer = k8sInformer.Services()
		serviceLister   = serviceInformer.Lister()

		controlplaneController = &Controller{
			k8sClient:     k8sClient,
			k8sInformers:  k8sInformers,
			config:        config,
			control:       NewDefaultControl(k8sClient, k8sInformer, recorder, serviceLister, config, lookupCh),
			recorder:      recorder,
			serviceLister: serviceLister,
			serviceQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "service"),
			workerCh:      make(chan int),
		}
	)

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controlplaneController.serviceAdd,
		UpdateFunc: controlplaneController.serviceUpdate,
		DeleteFunc: controlplaneController.serviceDelete,
	})

	controlplaneController.serviceSynced = serviceInformer.Informer().HasSynced

	return controlplaneController
}

// Run runs the Controller until the given stop channel can be read from.
func (c *Controller) Run(controlplaneWorkers int, stopCh <-chan struct{}) {
	var waitGroup sync.WaitGroup
	logrus.Info("waiting for caches to sync")
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		logrus.Error("Timed out waiting for caches to sync")
		return
	}
	logrus.Info("caches to sync")

	// Count number of running workers.
	go func() {
		for {
			select {
			case res := <-c.workerCh:
				c.numberOfRunningWorkers += res
				logrus.Debugf("Current number of running Controlplane workers is %d", c.numberOfRunningWorkers)
			}
		}
	}()

	logrus.Info("Controlplane controller initialized.")

	for i := 0; i < controlplaneWorkers; i++ {
		utils.CreateWorker(c.serviceQueue, "Service", c.reconcileServiceKey, stopCh, &waitGroup, c.workerCh)
	}

	// Shutdown handling
	<-stopCh
	c.serviceQueue.ShutDown()

	for {
		var serviceQueueLength = c.serviceQueue.Len()
		if serviceQueueLength == 0 && c.numberOfRunningWorkers == 0 {
			logrus.Debug("No running Controlplane worker and no items left in the queues. Terminated Controlplane controller...")
			break
		}
		logrus.Debugf("Waiting for %d Controlplane worker(s) to finish (%d item(s) left in the queues)...", c.numberOfRunningWorkers, serviceQueueLength)
		time.Sleep(5 * time.Second)
	}

	waitGroup.Wait()
}

// RunningWorkers returns the number of running workers.
func (c *Controller) RunningWorkers() int {
	return c.numberOfRunningWorkers
}
