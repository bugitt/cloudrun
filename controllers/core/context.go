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

package core

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type Context interface {
	context.Context
	client.Client

	Info(msg string, keysAndValues ...interface{})
	Infof(format string, args ...interface{})
	Error(err error, msg string, keysAndValues ...interface{})
	Errorf(err error, format string, args ...interface{})

	Name() string
	Namespace() string
	NamespacedName() ktypes.NamespacedName

	CreateResource(obj client.Object, force bool) error
	GetResource(obj client.Object) (bool, error)
}

func NewContext(ctx context.Context, client client.Client, req ctrl.Request) Context {
	logger := ctrl.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	return &DefaultContext{
		Context:        ctx,
		Client:         client,
		Logger:         logger,
		namespacedName: req.NamespacedName,
	}
}

type DefaultContext struct {
	context.Context
	client.Client
	logr.Logger

	namespacedName ktypes.NamespacedName
}

func (ctx *DefaultContext) Name() string {
	return ctx.namespacedName.Name
}

func (ctx *DefaultContext) Namespace() string {
	return ctx.namespacedName.Namespace
}

func (ctx *DefaultContext) NamespacedName() ktypes.NamespacedName {
	return ctx.namespacedName
}

func (ctx *DefaultContext) Info(msg string, keysAndValues ...interface{}) {
	ctx.Logger.Info(msg, keysAndValues...)
}

func (ctx *DefaultContext) Infof(format string, args ...interface{}) {
	ctx.Logger.Info(fmt.Sprintf(format, args...))
}

func (ctx *DefaultContext) Error(err error, msg string, keysAndValues ...interface{}) {
	ctx.Logger.Error(err, msg, keysAndValues...)
}

func (ctx *DefaultContext) Errorf(err error, format string, args ...interface{}) {
	ctx.Logger.Error(err, fmt.Sprintf(format, args...))
}

func (ctx *DefaultContext) CreateResource(obj client.Object, force bool) error {
	placeHolder := &unstructured.Unstructured{}
	gvk, err := apiutil.GVKForObject(obj, ctx.Client.Scheme())
	if err != nil {
		ctx.Error(err, fmt.Sprintf("Failed to get GVK for %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		return errors.Wrap(err, fmt.Sprintf("failed to get GVK for %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
	}
	placeHolder.SetGroupVersionKind(gvk)
	err = ctx.Get(ctx, ktypes.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, placeHolder)
	var exist bool
	if err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to get %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		}
		exist = false
	} else {
		exist = true
	}
	if exist && !force {
		return nil
	} else if exist && force {
		if err := ctx.Delete(ctx, obj); err != nil {
			ctx.Error(err, fmt.Sprintf("Failed to delete %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
			return errors.Wrap(err, fmt.Sprintf("failed to delete %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		}
	}
	if err := ctx.Create(ctx, obj); err != nil {
		ctx.Error(err, fmt.Sprintf("Failed to create %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		return errors.Wrap(err, fmt.Sprintf("failed to create %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
	}
	return nil
}

func (ctx *DefaultContext) GetResource(obj client.Object) (bool, error) {
	if err := ctx.Get(ctx, ctx.namespacedName, obj); err != nil {
		if client.IgnoreNotFound(err) != nil {
			ctx.Error(err, fmt.Sprintf("Failed to get %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
			return false, errors.Wrap(err, fmt.Sprintf("failed to get %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		}
		return false, nil
	}
	return true, nil
}
