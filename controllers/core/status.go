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
)

func PublishStatus(ctx Context, obj types.CloudRunCRD, err error) {
	crdKind := obj.GetObjectKind().GroupVersionKind().String()
	status := obj.CommonStatus()
	if err != nil {
		status.Status = types.StatusFailed
		status.Message = err.Error()
	}
	ctx.Infof("try to update %s status(%s, %s)", crdKind, status.Status, status.Message)
	if err := ctx.Status().Update(ctx, obj); err != nil {
		ctx.Errorf(err, "failed to update %s status", crdKind)
	} else {
		ctx.Infof("update %s status successfully", crdKind)
	}
}
