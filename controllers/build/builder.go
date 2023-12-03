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

package build

import (
	"context"
	"path/filepath"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/types"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	contextTarName = "contextTar"
)

const (
	prepareContextTarVolumeName = "prepare-context-tar"
	workspaceVolumeName         = "workspace"

	prepareContextTarVolumeMountPath = "/prepare-context"
	workspaceVolumeMountPath         = "/workspace"
	pushSecretVolumeMountPath        = "/home/user/.docker/"
)

const (
	s3CmdImageName   = "harbor.service.internal:8081/library/ls3cmd:1.0.0"
	wgetImageName    = "harbor.service.internal:8081/library/wget:alpine"
	unzipImageName   = "harbor.service.internal:8081/library/go-unarr:0.1.6"
	gitImageName     = "harbor.service.internal:8081/library/bitnami-git:2.39.0"
	builderImageName = "harbor.service.internal:8081/library/buildkit:this-version"
)

const (
	defaultPushSecretName = "push-secret"
)

var (
	backoffLimit int32 = 0
)

var (
	contextTarFileFullPath = filepath.Join(prepareContextTarVolumeMountPath, contextTarName)
)

type Context struct {
	core.Context
	Builder *v1alpha1.Builder
}

func NewContext(originCtx context.Context, cli client.Client, logger logr.Logger, builder *v1alpha1.Builder) *Context {
	ownerReference := apimetav1.OwnerReference{
		APIVersion:         builder.APIVersion,
		Kind:               builder.Kind,
		Name:               builder.Name,
		UID:                builder.GetUID(),
		BlockOwnerDeletion: core.Ptr(true),
	}
	defaultCtx := &core.DefaultContext{
		Context:            originCtx,
		Client:             cli,
		Logger:             logger,
		NamespacedNameVar:  types.NamespacedName{Namespace: builder.Namespace, Name: builder.Name},
		GetMasterResource:  func() (client.Object, error) { return builder, nil },
		GetOwnerReferences: func() (apimetav1.OwnerReference, error) { return ownerReference, nil },
	}
	return &Context{
		Context: defaultCtx,
		Builder: builder,
	}
}

func (ctx *Context) currentRound() int {
	return ctx.Builder.Status.Base.CurrentRound
}

func workspaceMount() apiv1.VolumeMount {
	return apiv1.VolumeMount{
		Name:      workspaceVolumeName,
		MountPath: workspaceVolumeMountPath,
	}
}

func (ctx *Context) BackupState() error {
	return core.BackupState(ctx.Builder, ctx.Builder.Spec)
}

func (ctx *Context) TriggerDeployer() error {
	for _, hook := range ctx.Builder.Spec.DeployerHooks {
		deployer := new(v1alpha1.Deployer)
		if err := ctx.Get(ctx.Context, ktypes.NamespacedName{Namespace: ctx.Builder.Namespace, Name: hook.DeployerName}, deployer); err != nil {
			if client.IgnoreNotFound(err) == nil {
				continue
			}
			return errors.Wrapf(err, "get deployer %s when try to trigger it", hook.DeployerName)
		}
		if hook.ForceRound {
			deployer.Spec.Round = ctx.Builder.GetRound()
		} else {
			deployer.Spec.Round = deployer.CommonStatus().CurrentRound + 1
		}
		if hook.DynamicImage {
			deployer.Spec.Containers[0].Image = ctx.Builder.Spec.Destination
		} else {
			deployer.Spec.Containers[0].Image = hook.Image
		}
		deployer.Spec.ResourcePool = hook.ResourcePool
		if err := ctx.Update(ctx.Context, deployer); err != nil {
			return errors.Wrapf(err, "update deployer %s when try to trigger it", hook.DeployerName)
		}
	}
	return nil
}

func (ctx *Context) NewJob() (*batchv1.Job, error) {
	builder := ctx.Builder

	dockerfilePath := builder.Spec.DockerfilePath
	if builder.Spec.Context.Raw != nil {
		dockerfilePath = uuid.NewString() + ".Dockerfile"
	}

	// workspaceVolume is an emptyDir to store the context dir
	workspaceVolume := apiv1.Volume{
		Name: workspaceVolumeName,
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{},
		},
	}

	pushSecretName := builder.Spec.PushSecretName
	if pushSecretName == "" {
		pushSecretName = defaultPushSecretName
	}
	pushSecretVolume := apiv1.Volume{
		Name: "push-secret",
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: pushSecretName,
				Items: []apiv1.KeyToPath{
					{
						Key:  ".dockerconfigjson",
						Path: "config.json",
					},
				},
			},
		},
	}
	pushSecretVolumeMount := apiv1.VolumeMount{
		Name:      pushSecretVolume.Name,
		MountPath: pushSecretVolumeMountPath,
	}

	podSpec := apiv1.PodSpec{}

	mainContainer := apiv1.Container{
		Name:  "main",
		Image: builderImageName,
		Env: []apiv1.EnvVar{
			{
				Name:  "BUILDKITD_FLAGS",
				Value: "--oci-worker-no-process-sandbox",
			},
			{
				Name:  "DOCKER_CONFIG",
				Value: pushSecretVolumeMountPath,
			},
		},
		Command: []string{"buildctl-daemonless.sh"},
		Args: []string{
			"build",
			"--frontend",
			"dockerfile.v0",
			"--local",
			"context=" + filepath.Join("/workspace", builder.Spec.WorkspacePath),
			"--local",
			"dockerfile=" + filepath.Join("/workspace", builder.Spec.WorkspacePath),
			"--output",
			"type=image,name=" + builder.Spec.Destination + ",push=true",
			"--opt",
			"filename=" + dockerfilePath,
		},
		SecurityContext: &apiv1.SecurityContext{
			Privileged: core.Ptr(true),
		},

		VolumeMounts: []apiv1.VolumeMount{workspaceMount(), pushSecretVolumeMount},
	}

	podSpec.Volumes = []apiv1.Volume{workspaceVolume, pushSecretVolume}
	podSpec.Containers = []apiv1.Container{mainContainer}
	podSpec.RestartPolicy = "Never"
	// podSpec.NodeSelector = map[string]string{
	// 	"scs.buaa.edu.cn/network": "true",
	// }

	switch {
	case builder.Spec.Context.S3 != nil:
		if err := ctx.addS3InitContainers(builder.Spec.Context.S3, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add s3 init containers")
		}

	case builder.Spec.Context.Git != nil:
		if err := ctx.addGitInitContainers(builder.Spec.Context.Git, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add git init containers")
		}

	case builder.Spec.Context.HTTP != nil:
		if err := ctx.addHTTPInitContainers(builder.Spec.Context.HTTP, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add http init containers")
		}
	}

	if builder.Spec.Context.Raw != nil {
		if err := ctx.addRawDockerfileInitContainers(*(builder.Spec.Context.Raw), &podSpec, dockerfilePath); err != nil {
			return nil, errors.Wrap(err, "add raw init containers")
		}
	}

	round := ctx.currentRound()
	job := &batchv1.Job{
		ObjectMeta: ctx.NewObjectMeta(round),
		Spec: batchv1.JobSpec{
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: ctx.NewObjectMeta(round),
				Spec:       podSpec,
			},
			BackoffLimit: &backoffLimit,
		},
	}
	return job, nil
}
