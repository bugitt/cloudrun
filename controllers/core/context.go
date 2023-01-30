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

	"github.com/bugitt/cloudrun/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
	NamespacedName() types.NamespacedName
	NewObjectMeta(round int) apimetav1.ObjectMeta

	GetServiceLabels() map[string]string

	CreateResource(obj client.Object, force, update bool) error
	GetSubResource(obj client.Object, round int) (bool, error)
}

type DefaultContext struct {
	context.Context
	client.Client
	logr.Logger

	NamespacedNameVar types.NamespacedName

	GetMasterResource  func() (client.Object, error)
	GetOwnerReferences func() (apimetav1.OwnerReference, error)
}

func (ctx *DefaultContext) Name() string {
	return ctx.NamespacedNameVar.Name
}

func (ctx *DefaultContext) Namespace() string {
	return ctx.NamespacedNameVar.Namespace
}

func (ctx *DefaultContext) NamespacedName() types.NamespacedName {
	return ctx.NamespacedNameVar
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

func setOwnerReference(obj client.Object, ownerRef apimetav1.OwnerReference) {
	refList := obj.GetOwnerReferences()
	for _, ref := range refList {
		if ref.UID == ownerRef.UID {
			return
		}
	}
	refList = append(refList, ownerRef)
	obj.SetOwnerReferences(refList)
}

func (ctx *DefaultContext) CreateResource(obj client.Object, force, update bool) error {
	// set the owner reference
	ownerRef, err := ctx.GetOwnerReferences()
	if err != nil {
		ctx.Error(err, "failed get owner reference")
		return errors.Wrap(err, "failed get shi")
	}
	setOwnerReference(obj, ownerRef)

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
	if exist {
		if update {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err = ctx.Get(ctx, ktypes.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, placeHolder)
				if err != nil {
					return err
				}
				obj.SetResourceVersion(placeHolder.GetResourceVersion())
				return ctx.Update(ctx, obj)
			})
			if err != nil {
				ctx.Error(err, fmt.Sprintf("Failed to update %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
				return errors.Wrap(err, fmt.Sprintf("failed to update %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
			}
			return nil
		}

		if force {
			if err := ctx.Delete(ctx, obj); err != nil {
				ctx.Error(err, fmt.Sprintf("Failed to delete %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
				return errors.Wrap(err, fmt.Sprintf("failed to delete %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
			}
		} else {
			return nil
		}
	}
	if err := ctx.Create(ctx, obj); err != nil {
		ctx.Error(err, fmt.Sprintf("failed to create %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		return errors.Wrap(err, fmt.Sprintf("failed to create %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
	}
	return nil
}

func (ctx *DefaultContext) GetSubResource(obj client.Object, round int) (bool, error) {
	name := fmt.Sprintf("%s-%d", ctx.Name(), round)
	if err := ctx.Get(ctx, ktypes.NamespacedName{Namespace: ctx.Namespace(), Name: name}, obj); err != nil {
		if client.IgnoreNotFound(err) != nil {
			ctx.Error(err, fmt.Sprintf("Failed to get %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
			return false, errors.Wrap(err, fmt.Sprintf("failed to get %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName()))
		}
		return false, nil
	}
	return true, nil
}

func (ctx *DefaultContext) GetServiceLabels() map[string]string {
	return map[string]string{
		"app": ctx.Name(),
	}
}

func (ctx *DefaultContext) NewObjectMeta(round int) apimetav1.ObjectMeta {
	ownerRefList := []apimetav1.OwnerReference{}
	if ownerRef, err := ctx.GetOwnerReferences(); err != nil {
		ctx.Error(err, "failed get owner reference")
	} else {
		ownerRefList = append(ownerRefList, ownerRef)
	}

	name := fmt.Sprintf("%s-%d", ctx.Name(), round)
	labels := ctx.GetServiceLabels()
	labels["owner.name"] = ctx.Name()
	labels["round"] = fmt.Sprintf("%d", round)

	return apimetav1.ObjectMeta{
		Name:            name,
		Namespace:       ctx.Namespace(),
		Labels:          labels,
		OwnerReferences: ownerRefList,
	}
}
