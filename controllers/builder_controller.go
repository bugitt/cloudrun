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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/build"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/controllers/finalize"
	"github.com/bugitt/cloudrun/types"
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

func (r *BuilderReconciler) Reconcile(originalCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(originalCtx, "builder", req.NamespacedName)
	builder := &cloudapiv1alpha1.Builder{}

	err := r.Get(originalCtx, req.NamespacedName, builder)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("Builder resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Builder.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get builder")
	}

	ctx := build.NewContext(
		originalCtx,
		r.Client,
		logger,
		builder,
	)

	defer func() {
		if re := recover(); re != nil {
			logger.Error(re.(error), "Reconcile panic")
			core.PublishStatus(ctx, builder, re.(error))
		}
	}()

	isToBeDeleted := builder.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(builder) {
			if err := r.finalizeBuilder(ctx); err != nil {
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

	if builder.Spec.Round < builder.CommonStatus().CurrentRound {
		ctx.Info("Builder is not ready. Ignoring.")
		builder.CommonStatus().Status = types.StatusUNDO
		return ctrl.Result{}, r.Status().Update(originalCtx, builder)
	}

	if builder.Spec.Round > builder.CommonStatus().CurrentRound {
		// should cleanup the current job and back up the current state
		if err := core.DeleteJob(ctx, builder.Status.Base.CurrentRound); err != nil {
			ctx.Error(err, "Failed to delete job")
			return ctrl.Result{}, errors.Wrap(err, "failed to delete job")
		}
		if err := ctx.BackupState(); err != nil {
			ctx.Error(err, "Failed to backup state")
			return ctrl.Result{}, errors.Wrap(err, "failed to backup state")
		}
		builder.CommonStatus().Status = types.StatusPending
		builder.CommonStatus().CurrentRound = builder.Spec.Round
		builder.CommonStatus().StartTime = time.Now().Unix()
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(originalCtx, builder)
	}

	ctx.FixBuilder()

	// create and exec image build job
	if err := r.createAndWatchJob(ctx); err != nil {
		ctx.Error(err, "Failed to setup builder job.")
		err = errors.Wrap(err, "failed to setup builder job")
		core.PublishStatus(ctx, builder, err)
		return ctrl.Result{}, err
	}

	core.PublishStatus(ctx, builder, nil)

	if !core.IsDoneOrFailed(builder) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	ctx.Info("Builder job has been done or failed. Ignoring.")
	return ctrl.Result{}, nil
}

func (r *BuilderReconciler) finalizeBuilder(ctx core.Context) error {
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
	// do nothing. As we have already set the ownerReference of the job and other resources as the builder,
	// the apiserver can help us to delete the sub resources automatically.
	return nil
}

func (r *BuilderReconciler) createAndWatchJob(ctx *build.Context) error {
	return core.CreateAndWatchJob(
		ctx,
		ctx.Builder,
		func() (*batchv1.Job, error) {
			return ctx.NewJob()
		},
		false,
	)
}
