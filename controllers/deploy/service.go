/*
Copyright 2022 SCS Team of School of Software, BUAA.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deploy

import (
	"time"

	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (ctx *Context) handleService() error {
	deployer := ctx.Deployer

	// 1. check if the service is already running
	deployment := new(appsv1.Deployment)
	exist, err := ctx.GetSubResource(deployment, ctx.currentRound())
	if err != nil {
		return errors.Wrap(err, "failed to get deployment for service type")
	}
	// 2. if not, create a new deployment and service
	if !exist {
		deployer.CommonStatus().Status = types.StatusPending
		core.PublishStatus(ctx, deployer, nil)

		if err := ctx.createDeployment(); err != nil {
			return errors.Wrap(err, "failed to create deployment for service type")
		}
		return nil
	}

	if err := ctx.createOrUpdateService(); err != nil {
		return errors.Wrap(err, "failed to create or update service for service type")
	}

	// 3. check if the service is ready
	status, message, err := core.GetStatusFromPod(ctx, deployment.Spec.Selector)
	if err != nil {
		return errors.Wrap(err, "failed to get status from pod for service type")
	}
	deployer.CommonStatus().Status = status
	deployer.CommonStatus().Message = message
	if status == types.StatusDone || status == types.StatusFailed || status == types.StatusDoing {
		deployer.CommonStatus().EndTime = time.Now().Unix()
	}

	return nil
}

func (ctx *Context) deleteDeployment() error {
	deployment := &appsv1.Deployment{}
	if exist, err := ctx.GetSubResource(deployment, ctx.currentRound()); err != nil {
		return err
	} else if exist {
		if err := ctx.Delete(ctx, deployment); err != nil {
			return errors.Wrap(err, "failed to delete deployment for service type")
		}
	}
	return nil
}

func (ctx *Context) createOrUpdateService() error {
	service := new(corev1.Service)
	exist, err := ctx.GetSubResource(service, ctx.currentRound())
	if err != nil {
		return errors.Wrap(err, "failed to get service for service type")
	}

	if !exist {
		service = &corev1.Service{
			ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
			Spec: corev1.ServiceSpec{
				Selector: ctx.GetServiceLabels(),
				Ports:    make([]corev1.ServicePort, 0),
				Type:     corev1.ServiceTypeNodePort,
			},
		}
	}

	searchInSvcPorts := func(p apiv1.ContainerPort) *corev1.ServicePort {
		for _, port := range service.Spec.Ports {
			if port.Name == p.Name {
				return port.DeepCopy()
			}
		}
		return nil
	}

	newSvcPorts := make([]corev1.ServicePort, 0)
	for _, container := range ctx.Deployer.Spec.Containers {
		for _, originPort := range container.Ports {
			if !originPort.Export {
				continue
			}
			containerPort := buildContainerPort(originPort)
			svcPort := searchInSvcPorts(containerPort)
			if svcPort == nil {
				svcPort = &corev1.ServicePort{
					Name:       containerPort.Name,
					Protocol:   containerPort.Protocol,
					Port:       containerPort.ContainerPort,
					TargetPort: intstr.FromInt(int(containerPort.ContainerPort)),
				}
			}
			newSvcPorts = append(newSvcPorts, *svcPort)
		}
	}
	service.Spec.Ports = newSvcPorts

	if exist {
		if err := ctx.Update(ctx, service); err != nil {
			return errors.Wrap(err, "failed to update service for service type")
		}
		return nil
	}

	return ctx.CreateResource(service, true)
}

func (ctx *Context) createDeployment() error {
	deployment := &appsv1.Deployment{
		ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
		Spec: appsv1.DeploymentSpec{
			Replicas: core.Ptr[int32](1),
			Selector: &apimetav1.LabelSelector{
				MatchLabels: ctx.GetServiceLabels(),
			},
			Template: ctx.newPodSpec(),
		},
	}
	return ctx.CreateResource(deployment, true)
}
