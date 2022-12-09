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

// +kubebuilder:validation:Enum=tar;tar.gz;zip;rar;dir
type ContextFileType string

const (
	TAR   ContextFileType = "tar"
	TARGZ ContextFileType = "tar.gz"
	ZIP   ContextFileType = "zip"
	RAR   ContextFileType = "rar"
	DIR   ContextFileType = "dir"
)

type BuilderContextLocalFS struct {
	Path string          `json:"path"`
	Type ContextFileType `json:"type"`
}

type BuildContextGit struct {
	// +kubebuilder:validation:Enum=http;https
	Scheme           string `json:"scheme"`
	EndpointWithPath string `json:"endpoint"`
	Username         string `json:"username,omitempty"`
	UserPassword     string `json:"userPassword,omitempty"`
}

type BuilderContext struct {
	LocalFS *BuilderContextLocalFS `json:"localFS,omitempty"`
	Git     *BuildContextGit       `json:"git,omitempty"`
	Raw     *string                `json:"raw,omitempty"`
}

// BuilderSpec defines the desired state of Builder
type BuilderSpec struct {
	Context        BuilderContext `json:"context"`
	DockerfilePath string         `json:"dockerfilePath"`
	Destination    string         `json:"destination"`
}

// BuilderStatus defines the observed state of Builder
type BuilderStatus struct {
	StatusWithMessage `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Builder is the Schema for the builders API
type Builder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuilderSpec   `json:"spec,omitempty"`
	Status BuilderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuilderList contains a list of Builder
type BuilderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Builder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Builder{}, &BuilderList{})
}
