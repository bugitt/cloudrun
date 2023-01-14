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
	"path"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func dockerfileConfigmapName(ctx core.Context) string {
	return ctx.Name() + "-dockerfile"
}

func (ctx *Context) addRawDockerfileInitContainers(dockerfile string, podSpec *apiv1.PodSpec, dockerfilePath string) error {
	const dockerfileVolumeName = "dockerfile"
	configmap := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dockerfileConfigmapName(ctx),
			Namespace: ctx.Namespace(),
		},
		Immutable: core.Ptr(true),
		Data: map[string]string{
			dockerfilePath: dockerfile,
		},
	}
	if err := ctx.CreateResource(configmap, true); err != nil {
		return errors.Wrap(err, "create configmap for dockerfile")
	}

	dockerfileVolume := apiv1.Volume{
		Name: dockerfileVolumeName,
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: dockerfileConfigmapName(ctx),
				},
			},
		},
	}
	dockerfileVolumeMount := apiv1.VolumeMount{
		Name:      dockerfileVolumeName,
		MountPath: path.Join(workspaceVolumeMountPath, dockerfilePath),
		SubPath:   dockerfilePath,
	}
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, dockerfileVolumeMount)
	podSpec.Volumes = append(podSpec.Volumes, dockerfileVolume)
	return nil
}

func DeleteDockerfileMapConfigmap(ctx core.Context, builder *cloudapiv1alpha1.Builder) error {
	if builder.Spec.Context.Raw == nil {
		return nil
	}
	cm := &apiv1.ConfigMap{}
	if err := ctx.Get(ctx, types.NamespacedName{Name: dockerfileConfigmapName(ctx), Namespace: ctx.Namespace()}, cm); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return errors.Wrap(err, "failed get dockerfile configmap when try to delete dockerfile configmap")
	}
	if err := ctx.Delete(ctx, cm); err != nil {
		return errors.Wrap(err, "failed delete dockerfile configmap")
	}
	return nil
}
