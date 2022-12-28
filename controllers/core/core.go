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
	"encoding/json"

	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
)

func CheckChanged[T types.CloudRunCRD](
	ctx Context,
	obj T,
	isEqual func(T, T) bool,
) (bool, error) {
	historyList := obj.CommonStatus().HistoryList

	if len(historyList) == 0 {
		// new added
		objBytes, err := json.Marshal(obj)
		if err != nil {
			return false, errors.Wrap(err, "failed to marshal the CRD")
		}
		obj.CommonStatus().HistoryList = (append(historyList, string(objBytes)))

		if err := ctx.Status().Update(ctx, obj); err != nil {
			return false, errors.Wrap(err, "failed to update the CRD string to historyList of status")
		}
		return false, nil
	}

	oldObjInterface, err := obj.GetDecoder()(historyList[len(historyList)-1])
	if err != nil {
		return false, err
	}
	oldObj := oldObjInterface.(T)

	// compare the new and old spec
	if isEqual(oldObj, obj) {
		return false, nil
	}

	// not equal, need to update
	newBuilderBytes, err := json.Marshal(obj)
	if err != nil {
		return true, errors.Wrap(err, "failed to marshal the builder spec")
	}
	obj.CommonStatus().HistoryList = (append(historyList, string(newBuilderBytes)))
	return true, ctx.Status().Update(ctx, obj)
}
