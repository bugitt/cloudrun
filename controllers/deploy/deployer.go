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

package deploy

import (
	"context"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/go-logr/logr"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	core.Context
	Deployer *v1alpha1.Deployer
}

func NewContext(originCtx context.Context, cli client.Client, logger logr.Logger, deployer *v1alpha1.Deployer) *Context {
	ownerReference := apimetav1.OwnerReference{
		APIVersion:         deployer.APIVersion,
		Kind:               deployer.Kind,
		Name:               deployer.Name,
		UID:                deployer.GetUID(),
		BlockOwnerDeletion: core.BoolPtr(true),
	}
	defaultCtx := &core.DefaultContext{
		Context:            originCtx,
		Client:             cli,
		Logger:             logger,
		NamespacedNameVar:  ktypes.NamespacedName{Namespace: deployer.Namespace, Name: deployer.Name},
		GetMasterResource:  func() (client.Object, error) { return deployer, nil },
		GetOwnerReferences: func() (apimetav1.OwnerReference, error) { return ownerReference, nil },
	}
	return &Context{
		Context:  defaultCtx,
		Deployer: deployer,
	}
}
