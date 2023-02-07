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
package types

type Status string

const (
	//StatusUNDO means the CRD is not deployed.
	StatusUNDO Status = "UNDO"
	// StatusPending means the CRD is pending to be deployed.
	StatusPending Status = "Pending"
	// StatusDoing means the CRD is being deployed.
	StatusDoing Status = "Doing"
	// StatusDone means the CRD has been deployed.
	StatusDone Status = "Done"
	// StatusFailed means the CRD failed to be deployed.
	StatusFailed Status = "Failed"
	// StatusUnknown means the CRD status is unknown.
	StatusUnknown Status = "Unknown"
)

type PodWorker struct {
	Name              string   `json:"name"`
	ContainerList     []string `json:"containerList"`
	InitContainerList []string `json:"initContainerList"`
}

type CommonStatus struct {
	Status Status `json:"status"`
	// Message is mainly used to store the error message when the CRD is failed.
	Message string `json:"message,omitempty"`
	// HistoryList is used to store the history of the CRD.
	HistoryList  []string   `json:"historyList,omitempty"`
	CurrentRound int        `json:"currentRound"`
	StartTime    int64      `json:"startTime,omitempty"`
	EndTime      int64      `json:"endTime,omitempty"`
	PodWorker    *PodWorker `json:"podWorker,omitempty"`
}

func (s *CommonStatus) HasStarted() bool {
	return s.Status != StatusUNDO && s.Status != StatusPending
}

func (s *CommonStatus) HashDone() bool {
	return s.Status == StatusDone || s.Status == StatusFailed
}

func (in *PodWorker) DeepCopyInto(out *PodWorker) {
	*out = *in
	if in.ContainerList != nil {
		in, out := &in.ContainerList, &out.ContainerList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.InitContainerList != nil {
		in, out := &in.InitContainerList, &out.InitContainerList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *PodWorker) DeepCopy() *PodWorker {
	if in == nil {
		return nil
	}
	out := new(PodWorker)
	in.DeepCopyInto(out)
	return out
}

func (in *CommonStatus) DeepCopyInto(out *CommonStatus) {
	*out = *in
	if in.HistoryList != nil {
		in, out := &in.HistoryList, &out.HistoryList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.PodWorker != nil {
		in, out := &in.PodWorker, &out.PodWorker
		*out = new(PodWorker)
		(*in).DeepCopyInto(*out)
	}
}

func (in *CommonStatus) DeepCopy() *CommonStatus {
	if in == nil {
		return nil
	}
	out := new(CommonStatus)
	in.DeepCopyInto(out)
	return out
}
