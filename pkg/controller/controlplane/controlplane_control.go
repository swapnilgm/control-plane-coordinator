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
	"encoding/json"
	"time"

	"github.com/gardener/control-plane-coordinator/pkg/componentconfig"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	informers "k8s.io/client-go/informers/core/v1"
	kubecorev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

func (c *Controller) serviceAdd(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		logrus.Errorf("Couldn't get key for object %+v: %v", obj, err)
		return
	}
	c.serviceQueue.Add(key)
}

func (c *Controller) serviceUpdate(oldObj, newObj interface{}) {
	var (
		oldSeed       = oldObj.(*corev1.Service)
		newSeed       = newObj.(*corev1.Service)
		specChanged   = !apiequality.Semantic.DeepEqual(oldSeed.Spec, newSeed.Spec)
		statusChanged = !apiequality.Semantic.DeepEqual(oldSeed.Status, newSeed.Status)
	)

	if !specChanged && statusChanged {
		return
	}
	c.serviceAdd(newObj)
}

func (c *Controller) serviceDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		logrus.Errorf("Couldn't get key for object %+v: %v", obj, err)
		return
	}
	c.serviceQueue.Add(key)
}

func (c *Controller) reconcileServiceKey(key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	if name != c.config.Service {
		logrus.Debugf("[SERVICE RECONCILE] %s - skipping because Service name %s does not match service %s", name, c.config.Service)
		return nil
	}
	service, err := c.serviceLister.Services(c.config.WatchNamespace).Get(name)
	if apierrors.IsNotFound(err) {
		logrus.Debugf("[SERVICE RECONCILE] %s - skipping because Service has been deleted", key)
		return nil
	}
	if err != nil {
		logrus.Infof("[SERVICE RECONCILE] %s - unable to retrieve object from store: %v", key, err)
		return err
	}

	err = c.control.ReconcileService(service, key)
	if err != nil {
		c.serviceQueue.AddAfter(key, 15*time.Second)
	}
	return err
}

// ControlInterface implements the control logic for updating Seeds. It is implemented as an interface to allow
// for extensions that provide different semantics. Currently, there is only one implementation.
type ControlInterface interface {
	// ReconcileService implements the control logic for Seed creation, update, and deletion.
	// If an implementation returns a non-nil error, the invocation will be retried using a rate-limited strategy.
	// Implementors should sink any errors that they do not wish to trigger a retry, and they may feel free to
	// exit exceptionally at any point provided they wish the update to be re-run at a later point in time.
	ReconcileService(service *corev1.Service, key string) error
}

// NewDefaultControl returns a new instance of the default implementation ControlInterface that
// implements the documented semantics for Seeds. updater is the UpdaterInterface used
// to update the status of Seeds. You should use an instance returned from NewDefaultControl() for any
// scenario other than testing.
func NewDefaultControl(k8sClient kubernetes.Client, k8sInformers informers.Interface, recorder record.EventRecorder, serviceLister kubecorev1listers.ServiceLister, config *componentconfig.CoordinatorConfiguration, lookupCh chan<- bool) ControlInterface {
	return &defaultControl{k8sClient, k8sInformers, recorder, serviceLister, config, lookupCh}
}

type defaultControl struct {
	k8sClient     kubernetes.Client
	k8sInformers  informers.Interface
	recorder      record.EventRecorder
	serviceLister kubecorev1listers.ServiceLister
	config        *componentconfig.CoordinatorConfiguration
	lookupCh      chan<- bool
}

func (c *defaultControl) ReconcileService(obj *corev1.Service, key string) error {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return err
	}

	var (
		service        = obj.DeepCopy()
		serviceJSON, _ = json.Marshal(service)
		serviceLogger  = logrus.New()
	)

	serviceLogger.Infof("[SERVICE RECONCILE] %s", key)
	serviceLogger.Debugf(string(serviceJSON))
	if len(service.Status.LoadBalancer.Ingress) == 0 {
		serviceLogger.Infof("Loadbalancer ingress is still empty. Ignoring the update")
	} else {
		for index, lb := range service.Status.LoadBalancer.Ingress {
			serviceLogger.Infof("lb %d, %s", index, lb.String())
			if len(lb.IP) > 0 {
				c.lookupCh <- true
			} else if len(lb.Hostname) > 0 {
				c.lookupCh <- true
			}
		}
	}

	return nil
}
