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

type DeployType string

const (
	JOB     DeployType = "job"
	SERVICE DeployType = "service"
)

type Env struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Protocol string

const (
	TCP  Protocol = "tcp"
	UDP  Protocol = "udp"
	SCTP Protocol = "sctp"
)

type Port struct {
	Port int32 `json:"port"`
	//+kubebuilder:default:=tcp
	//+kubebuilder:validation:Enum=tcp;udp;sctp
	Protocol Protocol `json:"protocol,omitempty"`
	//+kubebuilder:default:=false
	Export bool `json:"export,omitempty"`
}

type ContainerResource struct {
	CPU    int32 `json:"cpu"`    // m, 1000m = 1 core
	Memory int32 `json:"memory"` // Mi, 1024Mi = 1Gi
}

type ContainerSpec struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Resource ContainerResource `json:"resource"`
	//+kubebuilder:default:=false
	Initial bool     `json:"initial,omitempty"`
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Envs    []Env    `json:"env,omitempty"`
	Ports   []Port   `json:"port,omitempty"`
}

// DeployerSpec defines the desired state of Deployer
type DeployerSpec struct {
	//+kubebuilder:validation:Enum=job;service
	DeployType DeployType `json:"deployType"`
	//+kubebuilder:validation:MinItems=1
	Containers []ContainerSpec `json:"containers"`
}

// DeployerStatus defines the observed state of Deployer
type DeployerStatus struct {
	StatusWithMessage `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=`.status.message`
//+kubebuilder:subresource:status

// Deployer is the Schema for the deployers API
type Deployer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeployerSpec   `json:"spec"`
	Status DeployerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DeployerList contains a list of Deployer
type DeployerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Deployer{}, &DeployerList{})
}
