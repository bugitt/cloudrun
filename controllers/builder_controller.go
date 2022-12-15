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

package controllers

import (
	"context"
	"time"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/build"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/controllers/finalize"
)

// BuilderReconciler reconciles a Builder object
type BuilderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *BuilderReconciler) Reconcile(originalCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(originalCtx)
	ctx := core.NewContext(originalCtx, r.Client, req)
	builder := &cloudapiv1alpha1.Builder{}

	err := r.Get(originalCtx, req.NamespacedName, builder)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			ctx.Info("Builder resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		ctx.Error(err, "Failed to get Builder.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get builder")
	}

	isToBeDeleted := builder.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(builder) {
			if err := r.finalizeBuilder(ctx, builder); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to finalize builder")
			}

			finalize.Remove(builder)
			err := r.Update(originalCtx, builder)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update builder")
			}
		}
		return ctrl.Result{}, nil
	}

	if !finalize.Contains(builder) {
		finalize.Add(builder)
		if err := r.Update(originalCtx, builder); err != nil {
			ctx.Error(err, "Failed to update Builder after add finalizer")
			return ctrl.Result{}, errors.Wrap(err, "failed to update builder")
		}
	}

	reCreateJob, err := build.CompareAndUpdateBuilderSpec(ctx, builder)
	if err != nil {
		err = errors.Wrap(err, "failed to compare and update builder spec")
		r.updateStatus(ctx, builder, err)
		ctx.Error(err, "failed to compare and update builder spec")
		return ctrl.Result{}, err
	}

	if reCreateJob {
		if err := r.deleteJob(ctx); err != nil {
			err = errors.Wrap(err, "failed to cleanup the job")
			r.updateStatus(ctx, builder, err)
			ctx.Error(err, "failed to cleanup the job")
			return ctrl.Result{}, err
		}
		builder.Status.Status = cloudapiv1alpha1.StatusPending
		r.updateStatus(ctx, builder, nil)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// create and exec image build job
	if err := r.createAndWatchJob(ctx, builder); err != nil {
		ctx.Error(err, "Failed to setup builder job.")
		return ctrl.Result{}, errors.Wrap(err, "failed to setup builder job")
	}

	if !r.isDoneOrFailed(builder) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	ctx.Info("Builder job has been done or failed. Ignoring.")
	return ctrl.Result{}, nil
}

func (r *BuilderReconciler) finalizeBuilder(ctx core.Context, _ *cloudapiv1alpha1.Builder) error {
	ctx.Info("Finalizing Builder")
	// the only thing we need to do is to delete the job
	return r.cleanup(ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudapiv1alpha1.Builder{}).
		Complete(r)
}

func (r *BuilderReconciler) cleanup(ctx core.Context) error {
	// 1. cleanup the job
	if err := r.deleteJob(ctx); err != nil {
		return err
	}
	return nil
}

func (r *BuilderReconciler) createAndWatchJob(ctx core.Context, builder *cloudapiv1alpha1.Builder) error {
	if r.isDoneOrFailed(builder) {
		return r.cleanup(ctx)
	}

	// 1. check if the job is already running
	job, err := r.getJob(ctx)
	if err != nil {
		return err
	}
	// 2. if not, create a new job
	if job == nil {
		builder.Status.Status = cloudapiv1alpha1.StatusPending
		r.updateStatus(ctx, builder, nil)

		job, err := build.NewJob(ctx, builder)
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
	podList := &apiv1.PodList{}
	posListOptions := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(job.Spec.Selector.MatchLabels),
	}
	if err := ctx.List(ctx, podList, posListOptions); err != nil {
		ctx.Error(err, "failed to list pod")
		return errors.Wrap(err, "failed to list pod")
	}
	if len(podList.Items) == 0 {
		builder.Status.Status = cloudapiv1alpha1.StatusPending
		r.updateStatus(ctx, builder, nil)
		return nil
	}
	pod := podList.Items[0]
	podStatus := pod.Status
	switch podStatus.Phase {
	case apiv1.PodPending:
		builder.Status.Status = cloudapiv1alpha1.StatusPending
		r.updateStatus(ctx, builder, nil)
		return nil
	case apiv1.PodRunning:
		builder.Status.Status = cloudapiv1alpha1.StatusDoing
		r.updateStatus(ctx, builder, nil)
		return nil
	case apiv1.PodFailed:
		builder.Status.Status = cloudapiv1alpha1.StatusFailed
		builder.Status.Message = podStatus.Message
		if err := r.cleanup(ctx); err != nil {
			return nil
		}
		r.updateStatus(ctx, builder, nil)
		return nil
	case apiv1.PodSucceeded:
		builder.Status.Status = cloudapiv1alpha1.StatusDone
		if err = r.cleanup(ctx); err != nil {
			return nil
		}
		r.updateStatus(ctx, builder, nil)
		return nil
	}
	return nil
}

func (r *BuilderReconciler) updateStatus(ctx core.Context, builder *cloudapiv1alpha1.Builder, err error) {
	if err != nil {
		builder.Status.Status = cloudapiv1alpha1.StatusFailed
		builder.Status.Message = err.Error()
	}
	ctx.Infof("try to update builder status(%s, %s)", builder.Status.Status, builder.Status.Message)
	if err := r.Status().Update(ctx, builder); err != nil {
		ctx.Error(err, "failed to update builder status")
	} else {
		ctx.Info("update builder status successfully")
	}
}

func (r *BuilderReconciler) isDoneOrFailed(builder *cloudapiv1alpha1.Builder) bool {
	return builder.Status.Status == cloudapiv1alpha1.StatusDone || builder.Status.Status == cloudapiv1alpha1.StatusFailed
}

func (r *BuilderReconciler) getJob(ctx core.Context) (*batchv1.Job, error) {
	job := new(batchv1.Job)
	exist, err := ctx.GetResource(job)
	if err != nil {
		return nil, err
	} else if !exist {
		return nil, nil
	}
	return job, nil
}

func (r *BuilderReconciler) deleteJob(ctx core.Context) error {
	// 1. delete job
	job, err := r.getJob(ctx)
	if err != nil {
		ctx.Error(err, "get job when cleanup builder")
		return errors.Wrap(err, "get job when cleanup builder")
	}
	if job != nil {
		if err := r.Delete(ctx, job); err != nil {
			ctx.Error(err, "delete job")
			return errors.Wrap(err, "delete job")
		}
	}
	// TODO the raw dockerfile case
	return nil
}
