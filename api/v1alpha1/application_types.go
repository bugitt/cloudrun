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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApplicationStage string

const (
	// ApplicationStagePending means the application is pending to be deployed.
	ApplicationStagePending ApplicationStage = "Pending"
	// ApplicationStageBuilding means the application is being built.
	ApplicationStageBuilding ApplicationStage = "Building"
	// ApplicationStageDeploying means the application is being deployed.
	// If the application is a job, this stage means the job is running.
	ApplicationStageDeploying ApplicationStage = "Deploying"
	// ApplicationStageServing means the application is serving.
	// If the application is a job, this stage means the job has finished.
	ApplicationStageServing ApplicationStage = "Serving"
)

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	Stage             ApplicationStage `json:"stage"`
	StatusWithMessage `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Application is the Schema for the applications API
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
