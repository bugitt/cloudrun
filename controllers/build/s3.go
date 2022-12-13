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
	"github.com/bugitt/cloudrun/controllers/core"
)

// addS3InitContainers adds needed containers and volumes to fetch context from s3 to the podTemplateSpec
// this container will copy context from s3 to local emptyDir,
// and the context file will be named to {{ prepareContextDir }}/{{ contextTarName }} (defined in build/builder.go)
func addS3InitContainers(ctx core.Context, s3ContextCfg *v1alpha1.BuilderContextS3, pod *apiv1.PodSpec) error {
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

	pod.Volumes = append(pod.Volumes, prepareContextTarVolume)

	mcCopyContainer := apiv1.Container{
		Name:  "fetch-s3-context",
		Image: s3CmdImageName,
		Env: []apiv1.EnvVar{
			{
				Name:  "AWS_ACCESS_KEY_ID",
				Value: s3ContextCfg.AccessKeyID,
			},
			{
				Name:  "AWS_SECRET_ACCESS_KEY",
				Value: s3ContextCfg.AccessSecretKey,
			},
		},
		Args: []string{
			fmt.Sprintf(
				`ls3md download  -region=%s  -endpoint=%s://%s  -bucket=%s -source=%s -target=%s`,
				s3ContextCfg.Region,
				s3ContextCfg.Scheme,
				s3ContextCfg.Endpoint,
				s3ContextCfg.Bucket,
				s3ContextCfg.ObjectKey,
				contextTarFileFullPath,
			),
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

	pod.InitContainers = append(pod.InitContainers, mcCopyContainer, unzipContainer)

	return nil
}
