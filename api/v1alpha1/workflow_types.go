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

package v1alpha1

import (
	"github.com/bugitt/cloudrun/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkflowStage string

const (
	// WorkflowStagePending means the application is pending to be deployed.
	WorkflowStagePending WorkflowStage = "Pending"
	// ApplicationStageBuilding means the application is being built.
	WorkflowStageBuilding WorkflowStage = "Building"
	// WorkflowStageDeploying means the application is being deployed.
	// If the application is a job, this stage means the job is running.
	WorkflowStageDeploying WorkflowStage = "Deploying"
	// WorkflowStageServing means the application is serving.
	// If the application is a job, this stage means the job has finished.
	WorkflowStageServing WorkflowStage = "Serving"
)

type BuildSpec struct {
	Context          BuilderContext `json:"context"`
	BaseImage        string         `json:"baseImage"`
	WorkingDir       *string        `json:"workingDir,omitempty"`
	Command          *string        `json:"command,omitempty"`
	RegistryLocation *string        `json:"registryLocation"`
	//+kubebuilder:default:=push-secret
	PushSecretName *string `json:"pushSecretName,omitempty"`
}

type FilePair struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type DeploySpec struct {
	ChangeEnv    bool              `json:"changeEnv,omitempty"`
	FilePair     *FilePair         `json:"filePair,omitempty"`
	BaseImage    *string           `json:"baseImage,omitempty"`
	Command      *string           `json:"command,omitempty"`
	ResourcePool string            `json:"resourcePool"`
	Resource     ContainerResource `json:"resource"`
	Envs         map[string]string `json:"env,omitempty"`
	WorkingDir   *string           `json:"workingDir,omitempty"`
	//+kubebuilder:validation:Enum=job;service
	Type        DeployType      `json:"type"`
	Ports       []Port          `json:"ports,omitempty"`
	SidecarList []ContainerSpec `json:"sidecarList,omitempty"`
}

// WorkflowSpec defines the desired state of Workflow
type WorkflowSpec struct {
	//+kubebuilder:default=-1
	Round int `json:"round,omitempty"`

	Build  *BuildSpec  `json:"build,omitempty"`
	Deploy *DeploySpec `json:"deploy"`
}

// WorkflowStatus defines the observed state of Workflow
type WorkflowStatus struct {
	Stage WorkflowStage       `json:"stage,omitempty"`
	Base  *types.CommonStatus `json:"base,omitempty"`
}

//+kubebuilder:printcolumn:name="Round",type="string",JSONPath=`.spec.round`
//+kubebuilder:printcolumn:name="CurrentRound",type="string",JSONPath=`.status.base.currentRound`
//+kubebuilder:printcolumn:name="Stage",type="string",JSONPath=`.status.stage`
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.base.status`
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=`.status.base.message`
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Workflow is the Schema for the workflows API
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec"`
	Status WorkflowStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkflowList contains a list of Workflow
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workflow{}, &WorkflowList{})
}

func (w *Workflow) CommonStatus() *types.CommonStatus {
	if w.Status.Base == nil {
		w.Status.Base = &types.CommonStatus{}
	}
	return w.Status.Base
}

func (w *Workflow) GetRound() int {
	return w.Spec.Round
}
