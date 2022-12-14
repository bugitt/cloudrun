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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetConfigMap(ctx Context, create bool) (*corev1.ConfigMap, error) {
	cm := new(corev1.ConfigMap)
	exist, err := ctx.GetResource(cm)
	if err != nil {
		return nil, err
	} else if exist {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		return cm, nil
	}

	if !create {
		return nil, nil
	}
	// create new one
	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ctx.Name(),
			Namespace: ctx.Namespace(),
		},
	}
	if err := ctx.Create(ctx, cm); err != nil {
		return nil, errors.Wrap(err, "failed to create configmap")
	}
	cm = new(corev1.ConfigMap)
	_, _ = ctx.GetResource(cm)
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	return cm, nil
}

func CleanupSpec(ctx Context, key string) error {
	cm, err := GetConfigMap(ctx, false)
	if err != nil {
		return errors.Wrap(err, "failed to get configmap when cleanup builder")
	}
	if cm == nil {
		return nil
	}
	delete(cm.Data, key)
	if err := ctx.Update(ctx, cm); err != nil {
		return errors.Wrap(err, "failed to update configmap when cleanup builder")
	}
	return nil
}
