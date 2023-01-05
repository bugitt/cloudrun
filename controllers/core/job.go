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
	batchv1 "k8s.io/api/batch/v1"
)

func CreateAndWatchJob[T types.CloudRunCRD](
	ctx Context,
	obj T,
	newJob func() (*batchv1.Job, error),
	checkJobChanged func() (bool, error),
	deleteJobAfterDone bool,
) error {
	reCreateJob, err := checkJobChanged()
	if err != nil {
		return errors.Wrap(err, "failed to compare and update old job spec")
	}

	if obj.CommonStatus().CurrentRound < obj.GetRound() {
		obj.CommonStatus().CurrentRound = obj.GetRound()
		PublishStatus(ctx, obj, nil)
		reCreateJob = true
	}

	round := obj.GetRound()

	if reCreateJob {
		if err := DeleteJob(ctx, round); err != nil {
			return errors.Wrap(err, "failed to cleanup the old job")
		}
		obj.CommonStatus().Status = types.StatusPending
		return nil
	}

	if IsDoneOrFailed(obj) && deleteJobAfterDone {
		return DeleteJob(ctx, round)
	}

	// 1. check if the job is already running
	job, err := GetJob(ctx, round)
	if err != nil {
		return err
	}
	// 2. if not, create a new job
	if job == nil {
		obj.CommonStatus().Status = types.StatusPending
		PublishStatus(ctx, obj, nil)

		job, err := newJob()
		if err != nil {
			return err
		}
		if err := ctx.CreateResource(job, false); err != nil {
			return err
		}
		return nil
	}

	// 3. watch the job status
	// as we will only create just on pod in the job,
	// we can use the status of pod to represent the status of job
	status, message, err := GetStatusFromPod(ctx, job.Spec.Selector)
	if err != nil {
		return errors.Wrap(err, "failed to get status from pod")
	}
	obj.CommonStatus().Status = status
	obj.CommonStatus().Message = message

	if IsDoneOrFailed(obj) && deleteJobAfterDone {
		if err := DeleteJob(ctx, round); err != nil {
			return errors.Wrap(err, "failed to cleanup the old job")
		}
	}
	return nil
}

func GetJob(ctx Context, round int) (*batchv1.Job, error) {
	job := new(batchv1.Job)
	exist, err := ctx.GetResource(job, round)
	if err != nil {
		return nil, err
	} else if !exist {
		return nil, nil
	}
	return job, nil
}

func DeleteJob(ctx Context, round int) error {
	// 1. delete job
	job, err := GetJob(ctx, round)
	if err != nil {
		ctx.Error(err, "get job when cleanup builder")
		return errors.Wrap(err, "get job when cleanup builder")
	}
	if job == nil {
		return nil
	}

	if err := ctx.Delete(ctx, job); err != nil {
		ctx.Error(err, "delete job")
		return errors.Wrap(err, "delete job")
	}
	return nil
}
