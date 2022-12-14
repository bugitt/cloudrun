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
	"strings"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
		NamespacedNameVar:  types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		GetMasterResource:  func() (client.Object, error) { return deployer, nil },
		GetOwnerReferences: func() (apimetav1.OwnerReference, error) { return ownerReference, nil },
	}
	return &Context{
		Context:  defaultCtx,
		Deployer: deployer,
	}
}

func (ctx *Context) currentRound() int {
	return ctx.Deployer.Status.Base.CurrentRound
}

func (ctx *Context) GetCurrentResourcePool() (*v1alpha1.ResourcePool, error) {
	resourcePoolName := ctx.Deployer.Status.ResourcePool
	if resourcePoolName == "" {
		return nil, nil
	}
	resourcePool := &v1alpha1.ResourcePool{}
	if err := ctx.Get(ctx, ktypes.NamespacedName{Name: resourcePoolName}, resourcePool); err != nil {
		return nil, errors.Wrap(err, "get resource pool")
	}
	return resourcePool, nil
}

func (ctx *Context) ReleaseResource() error {
	deployer := ctx.Deployer
	// release the resource usage
	resourcePool, err := ctx.GetCurrentResourcePool()
	if err != nil {
		return err
	}
	if resourcePool == nil {
		return nil
	}
	targetIndex := -1
	for i, usage := range resourcePool.Spec.Usage {
		if usage.TypeMeta.APIVersion == deployer.APIVersion && usage.TypeMeta.Kind == deployer.Kind {
			if usage.NamespacedName.Name == deployer.Name && usage.NamespacedName.Namespace == deployer.Namespace {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex == -1 {
		return nil
	}
	usage := resourcePool.Spec.Usage[targetIndex]
	resourcePool.Spec.Usage = append(resourcePool.Spec.Usage[:targetIndex], resourcePool.Spec.Usage[targetIndex+1:]...)
	resourcePool.Spec.Free.CPU = resourcePool.Spec.Free.CPU + (usage.Resource.CPU)
	resourcePool.Spec.Free.Memory = resourcePool.Spec.Free.Memory + (usage.Resource.Memory)
	return ctx.Update(ctx, resourcePool)
}

func (ctx *Context) BegResource() error {
	deployer := ctx.Deployer
	var (
		cpu    int32
		memory int32
	)
	for _, container := range deployer.Spec.Containers {
		resource := container.Resource
		cpu += resource.CPU
		memory += resource.Memory
	}
	resourcePool := &v1alpha1.ResourcePool{}
	if err := ctx.Get(ctx, ktypes.NamespacedName{Name: deployer.Spec.ResourcePool}, resourcePool); err != nil {
		return errors.Wrap(err, "get resource pool")
	}
	targetIndex := -1
	for i, usage := range resourcePool.Spec.Usage {
		if usage.TypeMeta.APIVersion == deployer.APIVersion && usage.TypeMeta.Kind == deployer.Kind {
			if usage.NamespacedName.Name == deployer.Name && usage.NamespacedName.Namespace == deployer.Namespace {
				targetIndex = i
				break
			}
		}
	}
	if targetIndex != -1 {
		usage := resourcePool.Spec.Usage[targetIndex]
		if resource := usage.Resource; resource.CPU == cpu && resource.Memory == memory {
			return nil
		}
		if ctx.ReleaseResource() != nil {
			return errors.New("release old resource when try to beg new resource")
		}
	}
	resourcePool.Spec.Usage = append(resourcePool.Spec.Usage, v1alpha1.ResourceUsage{
		TypeMeta: deployer.TypeMeta,
		NamespacedName: &types.NamespacedName{
			Namespace: deployer.Namespace,
			Name:      deployer.Name,
		},
		Resource: &types.Resource{
			CPU:    cpu,
			Memory: memory,
		},
	})
	resourcePool.Spec.Free.CPU = resourcePool.Spec.Free.CPU - cpu
	resourcePool.Spec.Free.Memory = resourcePool.Spec.Free.Memory - memory
	if err := ctx.Update(ctx, resourcePool); err != nil {
		return errors.Wrap(err, "update resource pool")
	}
	deployer.Status.ResourcePool = deployer.Spec.ResourcePool
	return nil
}

func (ctx *Context) CleanupForNextRound() error {
	deployer := ctx.Deployer

	var err error
	switch deployer.Spec.Type {
	case v1alpha1.JOB:
		err = core.DeleteJob(ctx, ctx.currentRound())
	case v1alpha1.SERVICE:
		err = ctx.deleteDeployment()
	default:
		err = fmt.Errorf("unknown deployer type: %s", deployer.Spec.Type)
	}
	if err != nil {
		return err
	}
	if err := ctx.ReleaseResource(); err != nil {
		return err
	}
	ctx.Deployer.Status.ResourcePool = ""
	return nil
}

func (ctx *Context) BackupState() error {
	return core.BackupState(ctx.Deployer, ctx.Deployer.Spec)
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

func (ctx *Context) handleJob() error {
	return core.CreateAndWatchJob(
		ctx,
		ctx.Deployer,
		func() (*batchv1.Job, error) {
			return ctx.newJob(), nil
		},
		false,
	)
}

func (ctx *Context) newJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
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
		ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
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
