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

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
)

type CloudRunCRDSpec interface {
	v1alpha1.BuilderSpec | v1alpha1.DeployerSpec
}

type History[T CloudRunCRDSpec] struct {
	Round  int          `json:"round"`
	Status types.Status `json:"Status"`
	Spec   T            `json:"spec"`
}

func BackupState[T types.CloudRunCRD, S CloudRunCRDSpec](obj T, spec S) error {
	if obj.CommonStatus().CurrentRound == 0 {
		// no need to backup
		return nil
	}
	history := History[S]{
		Round:  obj.CommonStatus().CurrentRound,
		Status: obj.CommonStatus().Status,
		Spec:   spec,
	}
	jsonBytes, err := json.Marshal(history)
	if err != nil {
		return errors.Wrap(err, "marshal history")
	}
	obj.CommonStatus().HistoryList = append(obj.CommonStatus().HistoryList, string(jsonBytes))
	return nil
}
