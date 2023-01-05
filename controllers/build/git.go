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
	"net/url"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
)

func gitCommand(cfg *cloudapiv1alpha1.BuildContextGit) (string, error) {
	_, err := url.Parse(cfg.URLWithAuth)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse the git url")
	}
	gitCloneCmd := fmt.Sprintf("git clone %s %s", cfg.URLWithAuth, workspaceVolumeMountPath)
	if cfg.Ref == nil {
		return gitCloneCmd, nil
	}
	return fmt.Sprintf(
		"%s && cd %s && cd %s && git checkout %s",
		gitCloneCmd,
		workspaceVolumeMountPath,
		workspaceVolumeMountPath,
		*cfg.Ref,
	), nil
}

func (ctx *Context) addGitInitContainers(gitCfg *cloudapiv1alpha1.BuildContextGit, podSpec *apiv1.PodSpec) error {
	gitCmd, err := gitCommand(gitCfg)
	if err != nil {
		return errors.Wrap(err, "failed to generate git command")
	}
	gitCloneContainer := apiv1.Container{
		Name:         "git-clone",
		Image:        gitImageName,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{gitCmd},
		VolumeMounts: []apiv1.VolumeMount{workspaceMount()},
	}
	podSpec.InitContainers = append(podSpec.InitContainers, gitCloneContainer)
	return nil
}
