/*
Copyright 2023 SCS Team of School of Software, BUAA.

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

package workflow

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const workspaceAbsPath = "/workspace"

const dockerfileCompileStageTemplate = `
FROM {{.BaseImage}}
WORKDIR {{.WorkingDir}}
COPY . .
RUN {{.Command}}

`

const dockerfileBuildStageTemplate = `
FROM {{.BaseImage}}
COPY --from=0 {{.Source}} {{.Target}}
`

const displayName = "displayName"

type dockerfileCompileStageTemplateStruct struct {
	BaseImage  string
	WorkingDir string
	Command    string
}

type dockerfileBuildStageTemplateStruct struct {
	BaseImage  string
	WorkingDir string
	Source     string
	Target     string
	Command    string
}

type Context struct {
	core.Context
	Workflow *v1alpha1.Workflow
}

func NewContext(originCtx context.Context, cli client.Client, logger logr.Logger, workflow *v1alpha1.Workflow) *Context {
	ownerReference := apimetav1.OwnerReference{
		APIVersion:         workflow.APIVersion,
		Kind:               workflow.Kind,
		Name:               workflow.Name,
		UID:                workflow.GetUID(),
		BlockOwnerDeletion: core.Ptr(true),
	}
	defaultCtx := &core.DefaultContext{
		Context:            originCtx,
		Client:             cli,
		Logger:             logger,
		NamespacedNameVar:  types.NamespacedName{Namespace: workflow.Namespace, Name: workflow.Name},
		GetMasterResource:  func() (client.Object, error) { return workflow, nil },
		GetOwnerReferences: func() (apimetav1.OwnerReference, error) { return ownerReference, nil },
	}
	return &Context{
		Context:  defaultCtx,
		Workflow: workflow,
	}
}

func (ctx *Context) GenerateWorkload() error {
	workflow := ctx.Workflow
	if workflow.Spec.Build != nil {
		if builder, err := ctx.newBuilder(); err != nil {
			return err
		} else if err := ctx.CreateResource(builder, false, true); err != nil {
			return err
		}
	}

	if workflow.Spec.Deploy != nil {
		if deployer, err := ctx.newDeployer(); err != nil {
			return err
		} else if err := ctx.CreateResource(deployer, false, true); err != nil {
			return err
		}
	}
	return nil
}

func (ctx *Context) newBuilder() (*v1alpha1.Builder, error) {
	workflow := ctx.Workflow
	buildSpec := workflow.Spec.Build
	builder := &v1alpha1.Builder{
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      ctx.Name(),
			Namespace: ctx.Namespace(),
			Labels: map[string]string{
				"workflow": ctx.Workflow.Name,
			},
			Annotations: map[string]string{},
		},
	}
	if len(workflow.Annotations) > 0 && workflow.Annotations[displayName] != "" {
		builder.Annotations[displayName] = fmt.Sprintf("“%s”的镜像构建任务", workflow.Annotations[displayName])
	}

	builder.Spec.Context = buildSpec.Context
	raw, err := ctx.getDockerfile()
	if err != nil {
		return nil, err
	}
	builder.Spec.Context.Raw = &raw

	if buildSpec.WorkingDir != nil {
		builder.Spec.WorkspacePath = *buildSpec.WorkingDir
	}

	builder.Spec.Destination = fmt.Sprintf("%s/%s/%s:v-%d", *buildSpec.RegistryLocation, workflow.Namespace, workflow.Name, workflow.Spec.Round)

	if buildSpec.PushSecretName != nil {
		builder.Spec.PushSecretName = *buildSpec.PushSecretName
	}

	builder.Spec.DeployerHooks = []v1alpha1.DeployerHook{
		{
			DeployerName: ctx.Name(),
			ResourcePool: workflow.Spec.Deploy.ResourcePool,
			DynamicImage: true,
			ForceRound:   true,
		},
	}

	builder.Spec.Round = workflow.Spec.Round

	return builder, nil
}

func (ctx *Context) getDockerfile() (string, error) {
	workflow := ctx.Workflow
	workingDir := workspaceAbsPath
	if workflow.Spec.Build.WorkingDir != nil {
		workingDir = filepath.Join(workspaceAbsPath, *workflow.Spec.Build.WorkingDir)
	}
	compileCommand := "true"
	if workflow.Spec.Build.Command != nil {
		compileCommand = *workflow.Spec.Build.Command
	}
	data := &dockerfileCompileStageTemplateStruct{
		BaseImage:  workflow.Spec.Build.BaseImage,
		WorkingDir: workingDir,
		Command:    compileCommand,
	}
	compileTemplate := template.Must(template.New("compileDockerfileTemplate").Parse(dockerfileCompileStageTemplate))
	dockerfileCompileStageBuf := bytes.Buffer{}
	if err := compileTemplate.Execute(&dockerfileCompileStageBuf, data); err != nil {
		return "", errors.Wrap(err, "failed to build dockerfile")
	}
	dockerfile := fmt.Sprintf("%s\n", dockerfileCompileStageBuf.String())
	deploy := workflow.Spec.Deploy
	if deploy.ChangeEnv {
		if deploy.BaseImage == nil {
			return "", errors.New("base image is required when change env")
		}

		filePair := deploy.FilePair
		buildData := &dockerfileBuildStageTemplateStruct{
			BaseImage: *workflow.Spec.Deploy.BaseImage,
			Source:    filePair.Source,
			Target:    filePair.Target,
		}
		buildTemplate := template.Must(template.New("buildDockerfileTemplate").Parse(dockerfileBuildStageTemplate))
		dockerfileBuildStageBuf := bytes.Buffer{}
		if err := buildTemplate.Execute(&dockerfileBuildStageBuf, buildData); err != nil {
			return "", errors.Wrap(err, "failed to build dockerfile")
		}
		dockerfile += fmt.Sprintf("%s\n", dockerfileBuildStageBuf.String())
	}

	if deploy.Command != nil {
		dockerfile += fmt.Sprintf(`CMD %s`, *deploy.Command)
	}

	return dockerfile, nil
}

func (ctx *Context) newDeployer() (*v1alpha1.Deployer, error) {
	deploy := ctx.Workflow.Spec.Deploy
	deployer := &v1alpha1.Deployer{
		ObjectMeta: apimetav1.ObjectMeta{
			Name:      ctx.Name(),
			Namespace: ctx.Namespace(),
			Labels: map[string]string{
				"workflow": ctx.Workflow.Name,
			},
			Annotations: map[string]string{},
		},
	}
	if len(ctx.Workflow.Annotations) > 0 && ctx.Workflow.Annotations[displayName] != "" {
		deployer.Annotations[displayName] = fmt.Sprintf("“%s”的部署任务", ctx.Workflow.Annotations[displayName])
	}

	deployer.Spec.Type = deploy.Type
	deployer.Spec.ResourcePool = deploy.ResourcePool

	imageName := "fake-image"
	if ctx.Workflow.Spec.Build == nil {
		imageName = *deploy.BaseImage
	}

	container := v1alpha1.ContainerSpec{
		Name:     "main",
		Image:    imageName,
		Resource: deploy.Resource,
		Ports:    deploy.Ports,
		Envs:     deploy.Envs,
	}
	if deploy.WorkingDir != nil {
		container.WorkingDir = *deploy.WorkingDir
	}
	deployer.Spec.Containers = []v1alpha1.ContainerSpec{container}

	if sidecars := deploy.SidecarList; len(sidecars) != 0 {
		deployer.Spec.Containers = append(deployer.Spec.Containers, sidecars...)
	}

	if ctx.Workflow.Spec.Build == nil {
		deployer.Spec.Round = ctx.Workflow.Spec.Round
	}

	return deployer, nil
}
