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
	"fmt"

	apiv1 "k8s.io/api/core/v1"

	"github.com/bugitt/cloudrun/api/v1alpha1"
)

func (ctx *Context) addHTTPInitContainers(httpCfg *v1alpha1.BuildContextHTTP, podSpec *apiv1.PodSpec) error {
	// we also need an emptyDir to store the context tar file
	prepareContextTarVolume := apiv1.Volume{
		Name: prepareContextTarVolumeName,
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{},
		},
	}
	emptyDirVolumeMount := apiv1.VolumeMount{
		Name:      prepareContextTarVolume.Name,
		MountPath: prepareContextTarVolumeMountPath,
	}

	podSpec.Volumes = append(podSpec.Volumes, prepareContextTarVolume)

	wgetContainer := apiv1.Container{
		Name:    "wget",
		Image:   wgetImageName,
		Command: []string{"wget"},
		Args: []string{
			httpCfg.URL,
			"-O",
			contextTarFileFullPath,
		},
		VolumeMounts: []apiv1.VolumeMount{emptyDirVolumeMount},
	}

	// we need a container to unzip the context tar file
	unzipContainer := apiv1.Container{
		Name:         "unzip-context",
		Image:        unzipImageName,
		Command:      []string{"sh", "-c"},
		Args:         []string{fmt.Sprintf("unarr %s %s", contextTarFileFullPath, workspaceVolumeMountPath)},
		VolumeMounts: []apiv1.VolumeMount{emptyDirVolumeMount, workspaceMount()},
	}

	podSpec.InitContainers = append(podSpec.InitContainers, wgetContainer, unzipContainer)

	return nil
}
