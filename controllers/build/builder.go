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
	"encoding/json"
	"path/filepath"
	"reflect"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	contextTarName = "contextTar"
)

const (
	prepareContextTarVolumeName = "prepare-context-tar"
	workspaceVolumeName         = "workspace"

	prepareContextTarVolumeMountPath = "/prepare-context"
	workspaceVolumeMountPath         = "/workspace"
	pushSecretVolumeMountPath        = "/kaniko/.docker/"
)

const (
	s3CmdImageName  = "loheagn/ls3md:1.0.0"
	unzipImageName  = "loheagn/go-unarr:0.1.6"
	gitImageName    = "bitnami/git:2.39.0"
	kanikoImageName = "scs.buaa.edu.cn:8081/iobs/kaniko-executor"
)

const (
	defaultPushSecretName = "push-secret"
)

const (
	builderSpecKey = "builder-spec"
)

var (
	backoffLimit int32 = 0
)

var (
	contextTarFileFullPath = filepath.Join(prepareContextTarVolumeMountPath, contextTarName)
)

func workspaceMount() apiv1.VolumeMount {
	return apiv1.VolumeMount{
		Name:      workspaceVolumeName,
		MountPath: workspaceVolumeMountPath,
	}
}

func NewJob(ctx core.Context, builder *v1alpha1.Builder) (*batchv1.Job, error) {
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
		Image: kanikoImageName,
		Args: []string{
			"--dockerfile=" + builder.Spec.DockerfilePath,
			"--context=dir:///workspace",
			"--destination=" + builder.Spec.Destination,
		},
		VolumeMounts: []apiv1.VolumeMount{workspaceMount(), pushSecretVolumeMount},
	}

	podSpec.Volumes = []apiv1.Volume{workspaceVolume, pushSecretVolume}
	podSpec.Containers = []apiv1.Container{mainContainer}
	podSpec.RestartPolicy = "Never"

	switch {
	case builder.Spec.Context.S3 != nil:
		if err := addS3InitContainers(ctx, builder.Spec.Context.S3, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add s3 init containers")
		}

	case builder.Spec.Context.Git != nil:
		if err := addGitInitContainers(ctx, builder.Spec.Context.Git, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add git init containers")
		}

		// TODO other cases
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      builder.Name,
			Namespace: builder.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      builder.Name,
					Namespace: builder.Namespace,
				},
				Spec: podSpec,
			},
			BackoffLimit: &backoffLimit,
		},
	}
	return job, nil
}

// CompareAndUpdateBuilderSpec compares the builder spec stored in the existing configmap and update it.
// It returns whether the controller should recreate the job
func CompareAndUpdateBuilderSpec(ctx core.Context, spec cloudapiv1alpha1.BuilderSpec) (bool, error) {
	cm, err := core.GetConfigMap(ctx, true)
	if err != nil || cm == nil {
		return false, errors.Wrap(err, "failed to get configmap when get builder spec")
	}
	builderSpecStr, ok := cm.Data[builderSpecKey]

	if !ok {
		// new added
		builderSpecBytes, err := json.Marshal(spec)
		if err != nil {
			return false, errors.Wrap(err, "failed to marshal the builder spec")
		}
		cm.Data[builderSpecKey] = string(builderSpecBytes)
		if err := ctx.Update(ctx, cm); err != nil {
			return false, errors.Wrap(err, "failed to update the configmap")
		}
		return false, nil
	}

	oldSpec := new(cloudapiv1alpha1.BuilderSpec)
	if err := json.Unmarshal([]byte(builderSpecStr), oldSpec); err != nil {
		return false, errors.Wrap(err, "failed to unmarshal builderSpec when get builder spec")
	}

	// compare the new and old spec
	if isSpecEqual(oldSpec, &spec) {
		return false, nil
	}

	// not equal, need to update
	newSpecBytes, err := json.Marshal(spec)
	if err != nil {
		return true, errors.Wrap(err, "failed to marshal the builder spec")
	}
	cm.Data[builderSpecKey] = string(newSpecBytes)
	return true, ctx.Update(ctx, cm)
}

func isSpecEqual(oldSpec, newSpec *cloudapiv1alpha1.BuilderSpec) bool {
	return reflect.DeepEqual(oldSpec, newSpec)
}

func CleanupMeta(ctx core.Context) error {
	return core.CleanupSpec(ctx, builderSpecKey)
}
