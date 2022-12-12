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

type BuilderContextS3 struct {
	//+kubebuilder:default:=s3.amazonaws.com
	Endpoint string `json:"endpoint,omitempty"`
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	//+kubebuilder:validation:Enum=http;https
	Scheme          string `json:"scheme,omitempty"`
	AccessKeyID     string `json:"accessKeyID"`
	SecretAccessKey string `json:"secretAccessKey"`
	ObjectKey       string `json:"objectKey"`

	FileType ContextFileType `json:"fileType,omitempty"`
}

type BuildContextGit struct {
	//+kubebuilder:validation:Enum=http;https
	//+kubebuilder:default:=https
	Scheme           string `json:"scheme"`
	EndpointWithPath string `json:"endpoint"`
	Username         string `json:"username,omitempty"`
	UserPassword     string `json:"userPassword,omitempty"`
	Ref              string `json:"ref,omitempty"`
}

type BuilderContext struct {
	S3  *BuilderContextS3 `json:"s3,omitempty"`
	Git *BuildContextGit  `json:"git,omitempty"`
	Raw *string           `json:"raw,omitempty"`
}

// BuilderSpec defines the desired state of Builder
type BuilderSpec struct {
	Context        BuilderContext `json:"context"`
	DockerfilePath string         `json:"dockerfilePath"`
	Destination    string         `json:"destination"`
	//+kubebuilder:default:=push-secret
	PushSecretName string `json:"pushSecretName,omitempty"`
}

// BuilderStatus defines the observed state of Builder
type BuilderStatus struct {
	//+kubebuilder:default:=Pending
	OtherMessage string `json:"otherMessage,omitempty"`
	Status       Status `json:"status,omitempty"`
	Message      string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Other",type="string",JSONPath=`.status.otherMessage`
//+kubebuilder:printcolumn:name="Tt",type="string",JSONPath=`.spec.destination`
//+kubebuilder:subresource:status

// Builder is the Schema for the builders API
type Builder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuilderSpec   `json:"spec"`
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
