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
	"encoding/json"

	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
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
	AccessSecretKey string `json:"accessSecretKey"`
	ObjectKey       string `json:"objectKey"`

	FileType ContextFileType `json:"fileType,omitempty"`
}

type BuildContextGit struct {
	URLWithAuth string  `json:"urlWithAuth"`
	Ref         *string `json:"ref,omitempty"`
}

type BuilderContext struct {
	S3  *BuilderContextS3 `json:"s3,omitempty"`
	Git *BuildContextGit  `json:"git,omitempty"`
	Raw *string           `json:"raw,omitempty"`
}

type DeployerHook struct {
	DeployerName string `json:"deployerName"`
	ResourcePool string `json:"resourcePool"`
	Image        string `json:"image,omitempty"`
	DynamicImage bool   `json:"dynamicImage,omitempty"`
}

// BuilderSpec defines the desired state of Builder
type BuilderSpec struct {
	Context       BuilderContext `json:"context"`
	WorkspacePath string         `json:"workspacePath,omitempty"`
	//+kubebuilder:default:=Dockerfile
	DockerfilePath string `json:"dockerfilePath,omitempty"`
	Destination    string `json:"destination"`
	//+kubebuilder:default:=push-secret
	PushSecretName string `json:"pushSecretName,omitempty"`
	//+kubebuilder:default:=-1
	Round         int            `json:"round,omitempty"`
	DeployerHooks []DeployerHook `json:"deployerHooks,omitempty"`
}

// BuilderStatus defines the observed state of Builder
type BuilderStatus struct {
	Base *types.CommonStatus `json:"base,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Target",type="string",JSONPath=`.spec.destination`
//+kubebuilder:printcolumn:name="Round",type="string",JSONPath=`.spec.round`
//+kubebuilder:printcolumn:name="CurrentRound",type="string",JSONPath=`.status.base.currentRound`
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.base.status`
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=`.status.base.message`
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

func (b *Builder) GetDecoder() types.DecodeFunc {
	return func(str string) (types.CloudRunCRD, error) {
		obj := new(Builder)
		if err := json.Unmarshal([]byte(str), obj); err != nil {
			return nil, errors.Wrap(err, "unmarshal builder error")
		}
		return obj, nil
	}
}

func (b *Builder) CommonStatus() *types.CommonStatus {
	if b.Status.Base == nil {
		b.Status.Base = &types.CommonStatus{}
	}
	return b.Status.Base
}

func (b *Builder) GetRound() int {
	return b.Spec.Round
}
