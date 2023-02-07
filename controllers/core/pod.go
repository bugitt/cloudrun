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
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetStatusFromPod(ctx Context, selector *metav1.LabelSelector) (status types.Status, message string, pod *apiv1.Pod, err error) {
	podList := &apiv1.PodList{}
	posListOptions := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(selector.MatchLabels),
	}
	if err = ctx.List(ctx, podList, posListOptions); err != nil {
		ctx.Error(err, "failed to list pod")
		err = errors.Wrap(err, "failed to list pod")
		return
	}
	if len(podList.Items) == 0 {
		status = types.StatusPending
		return
	}
	thisPod := podList.Items[0]
	pod = &thisPod
	podStatus := pod.Status
	switch podStatus.Phase {
	case apiv1.PodPending:
		status = types.StatusPending
	case apiv1.PodRunning:
		status = types.StatusDoing
	case apiv1.PodFailed:
		status = types.StatusFailed
		message = podStatus.Message
		return
	case apiv1.PodSucceeded:
		status = types.StatusDone
	}
	return
}
