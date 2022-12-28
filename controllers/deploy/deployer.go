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
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	core.Context
	Deployer *v1alpha1.Deployer
}

func NewContext(originCtx context.Context, cli client.Client, logger logr.Logger, deployer *v1alpha1.Deployer) *Context {
	ownerReference := apimetav1.OwnerReference{
		APIVersion:         deployer.APIVersion,
		Kind:               deployer.Kind,
		Name:               deployer.Name,
		UID:                deployer.GetUID(),
		BlockOwnerDeletion: core.Ptr(true),
	}
	defaultCtx := &core.DefaultContext{
		Context:            originCtx,
		Client:             cli,
		Logger:             logger,
		NamespacedNameVar:  ktypes.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		GetMasterResource:  func() (client.Object, error) { return deployer, nil },
		GetOwnerReferences: func() (apimetav1.OwnerReference, error) { return ownerReference, nil },
	}
	return &Context{
		Context:  defaultCtx,
		Deployer: deployer,
	}
}

func (ctx *Context) Handle() error {
	deployer := ctx.Deployer

	switch deployer.Spec.Type {
	case v1alpha1.JOB:
		return ctx.handleJob()
	case v1alpha1.SERVICE:
		return ctx.handleService()
	default:
		return fmt.Errorf("unknown deployer type: %s", deployer.Spec.Type)
	}
}

func (ctx *Context) checkChanged() (bool, error) {
	return core.CheckChanged(
		ctx,
		ctx.Deployer,
		func(oldObj, newObj *v1alpha1.Deployer) bool {
			return reflect.DeepEqual(oldObj.Spec, newObj.Spec)
		},
	)
}

func (ctx *Context) handleJob() error {
	return core.CreateAndWatchJob(
		ctx,
		ctx.Deployer,
		func() (*batchv1.Job, error) {
			return ctx.newJob(), nil
		},
		func() (bool, error) {
			return ctx.checkChanged()
		},
		false,
	)
}

func (ctx *Context) newJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: ctx.NewObjectMeta(),
		Spec: batchv1.JobSpec{
			Template: ctx.newPodSpec(),
		},
	}
}

func (ctx *Context) newPodSpec() apiv1.PodTemplateSpec {
	containers := make([]apiv1.Container, 0)
	initContainers := make([]apiv1.Container, 0)

	newContainer := func(containerSpec v1alpha1.ContainerSpec) apiv1.Container {
		envVars := make([]apiv1.EnvVar, 0)
		for k, v := range containerSpec.Envs {
			envVars = append(envVars, apiv1.EnvVar{
				Name:  k,
				Value: v,
			})
		}

		resource := apiv1.ResourceRequirements{
			Limits: map[apiv1.ResourceName]resource.Quantity{
				apiv1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", containerSpec.Resource.CPU)),
				apiv1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", containerSpec.Resource.Memory)),
			},
		}

		return apiv1.Container{
			Name:            containerSpec.Name,
			Image:           containerSpec.Image,
			ImagePullPolicy: apiv1.PullAlways,
			Command:         containerSpec.Command,
			Args:            containerSpec.Args,
			Env:             envVars,
			Ports:           convertOriginPort2ContainerPort(containerSpec.Ports),
			Resources:       resource,
		}
	}

	for _, spec := range ctx.Deployer.Spec.Containers {
		container := newContainer(spec)
		if spec.Initial {
			initContainers = append(initContainers, container)
		} else {
			containers = append(containers, container)
		}
	}

	return apiv1.PodTemplateSpec{
		ObjectMeta: ctx.NewObjectMeta(),
		Spec: apiv1.PodSpec{
			InitContainers: initContainers,
			Containers:     containers,
		},
	}
}

func convertOriginPort2ContainerPort(originPorts []v1alpha1.Port) []apiv1.ContainerPort {
	ports := make([]apiv1.ContainerPort, 0)
	for _, port := range originPorts {
		ports = append(ports, buildContainerPort(port))
	}
	return ports
}

func buildContainerPort(port v1alpha1.Port) apiv1.ContainerPort {
	return apiv1.ContainerPort{
		Name:          fmt.Sprintf("%s-%d", strings.ToLower(string(port.Protocol)), port.Port),
		ContainerPort: port.Port,
		Protocol:      apiv1.Protocol(strings.ToUpper(string(port.Protocol))),
	}
}
