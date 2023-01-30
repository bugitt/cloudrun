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

type ResourceUsage struct {
	Resource       *types.Resource       `json:"resource"`
	TypeMeta       metav1.TypeMeta       `json:"typeMeta"`
	NamespacedName *types.NamespacedName `json:"namespacedName"`
	DisplayName    string                `json:"displayName"`
}

// ResourcePoolSpec defines the desired state of ResourcePool
type ResourcePoolSpec struct {
	Capacity *types.Resource `json:"capacity"`
}

// ResourcePoolStatus defines the observed state of ResourcePool
type ResourcePoolStatus struct {
	Usage []ResourceUsage `json:"usage,omitempty"`
	Free  *types.Resource `json:"free,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ResourcePool is the Schema for the resourcepools API
type ResourcePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourcePoolSpec   `json:"spec,omitempty"`
	Status ResourcePoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourcePoolList contains a list of ResourcePool
type ResourcePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourcePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourcePool{}, &ResourcePoolList{})
}
