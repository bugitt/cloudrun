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
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	core.Context
	Deployer *v1alpha1.Deployer

	GetOwnerReferences func() (apimetav1.OwnerReference, error)
}

func NewContext(originCtx context.Context, cli client.Client, logger logr.Logger, deployer *v1alpha1.Deployer) *Context {
	ownerReference := apimetav1.OwnerReference{
		APIVersion:         deployer.APIVersion,
		Kind:               deployer.Kind,
		Name:               deployer.Name,
		UID:                deployer.GetUID(),
		BlockOwnerDeletion: core.Ptr(true),
	}
	getOwnerReferences := func() (apimetav1.OwnerReference, error) { return ownerReference, nil }
	defaultCtx := &core.DefaultContext{
		Context:            originCtx,
		Client:             cli,
		Logger:             logger,
		NamespacedNameVar:  types.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		GetMasterResource:  func() (client.Object, error) { return deployer, nil },
		GetOwnerReferences: getOwnerReferences,
	}
	return &Context{
		Context:  defaultCtx,
		Deployer: deployer,

		GetOwnerReferences: getOwnerReferences,
	}
}

func (ctx *Context) currentRound() int {
	return ctx.Deployer.Status.Base.CurrentRound
}

func (ctx *Context) displayName() string {
	name := ctx.Deployer.ObjectMeta.Annotations["displayName"]
	if len(name) != 0 {
		return name
	}
	name = ctx.Deployer.ObjectMeta.Labels["displayName"]
	if len(name) != 0 {
		return name
	}
	return ctx.Deployer.Name
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
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// release the resource usage
		resourcePool, err := ctx.GetCurrentResourcePool()
		if err != nil {
			return err
		}
		if resourcePool == nil {
			return nil
		}
		targetIndex := -1
		for i, usage := range resourcePool.Status.Usage {
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
		resourcePool.Status.Usage = append(resourcePool.Status.Usage[:targetIndex], resourcePool.Status.Usage[targetIndex+1:]...)
		return ctx.Context.Status().Update(ctx, resourcePool)
	})
}

func (ctx *Context) BegResource() error {
	deployer := ctx.Deployer
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
		if resourcePool.Status.Free.CPU < cpu || resourcePool.Status.Free.Memory < memory {
			deployer.CommonStatus().Status = types.StatusFailed
			return errors.New("resource pool is not enough")
		}

		resourcePool.Status.Usage = append(resourcePool.Status.Usage, v1alpha1.ResourceUsage{
			TypeMeta: deployer.TypeMeta,
			NamespacedName: &types.NamespacedName{
				Namespace: deployer.Namespace,
				Name:      deployer.Name,
			},
			DisplayName: ctx.displayName(),
			Resource: &types.Resource{
				CPU:    cpu,
				Memory: memory,
			},
		})
		return ctx.Context.Status().Update(ctx, resourcePool)
	})
	if err != nil {
		return errors.Wrap(err, "beg resource")
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
		nil,
	)
}

func (ctx *Context) newJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
		Spec: batchv1.JobSpec{
			Template: ctx.newPodSpec(ctx.currentRound(), false),
		},
	}
}

func (ctx *Context) newPodSpec(round int, isService bool) apiv1.PodTemplateSpec {
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
			Requests: map[apiv1.ResourceName]resource.Quantity{
				apiv1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", containerSpec.Resource.CPU/2)),
				apiv1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", containerSpec.Resource.Memory/2)),
			},
		}

		return apiv1.Container{
			Name:            containerSpec.Name,
			Image:           containerSpec.Image,
			ImagePullPolicy: apiv1.PullAlways,
			WorkingDir:      containerSpec.WorkingDir,
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

	podSpec := apiv1.PodTemplateSpec{
		ObjectMeta: ctx.NewObjectMeta(ctx.currentRound()),
		Spec: apiv1.PodSpec{
			InitContainers: initContainers,
			Containers:     containers,
		},
	}
	if isService {
		podSpec.Labels["type"] = "service"
	} else {
		podSpec.Labels["type"] = "job"
	}
	return podSpec
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
