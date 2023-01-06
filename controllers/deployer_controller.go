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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/controllers/deploy"
	"github.com/bugitt/cloudrun/controllers/finalize"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
)

// DeployerReconciler reconciles a Deployer object
type DeployerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;delete;update;patch
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch

func (r *DeployerReconciler) Reconcile(originalCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(originalCtx, "builder", req.NamespacedName)
	deployer := &cloudapiv1alpha1.Deployer{}

	err := r.Get(originalCtx, req.NamespacedName, deployer)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("Deployer resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Deployer.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get deployer")
	}

	ctx := deploy.NewContext(
		originalCtx,
		r.Client,
		logger,
		deployer,
	)

	isToBeDeleted := deployer.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(deployer) {
			if err := r.finalizeDeployer(ctx); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to finalize deployer")
			}

			finalize.Remove(deployer)
			err := r.Update(originalCtx, deployer)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update deployer")
			}
		}
		return ctrl.Result{}, nil
	}

	if !finalize.Contains(deployer) {
		finalize.Add(deployer)
		if err := r.Update(originalCtx, deployer); err != nil {
			ctx.Error(err, "Failed to update deployer after add finalizer")
			return ctrl.Result{}, errors.Wrap(err, "failed to update deployer")
		}
	}

	if deployer.Spec.Round < deployer.CommonStatus().CurrentRound {
		ctx.Info("Deployer is not ready. Ignoring.")
		deployer.CommonStatus().Status = types.StatusUNDO
		return ctrl.Result{}, r.Status().Update(ctx, deployer)
	}

	if deployer.Spec.Round > deployer.CommonStatus().CurrentRound {
		if err := ctx.CleanupForNextRound(); err != nil {
			ctx.Error(err, "Failed to cleanup for next round")
			return ctrl.Result{}, errors.Wrap(err, "failed to cleanup for next round")
		}
		if err := ctx.BackupState(); err != nil {
			ctx.Error(err, "Failed to backup state")
			return ctrl.Result{}, errors.Wrap(err, "failed to backup state")
		}
		deployer.CommonStatus().Status = types.StatusPending
		deployer.CommonStatus().CurrentRound = deployer.Spec.Round
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, deployer)
	}

	if err := ctx.Handle(); err != nil {
		ctx.Error(err, "Failed to handle deployer")
		err = errors.Wrap(err, "failed to handle deployer")
		core.PublishStatus(ctx, deployer, err)
		return ctrl.Result{}, err
	}

	core.PublishStatus(ctx, deployer, nil)

	if !core.IsDoneOrFailed(deployer) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	ctx.Info("Deployer is done or failed. Ignoring.")

	return ctrl.Result{}, nil
}

func (r *DeployerReconciler) finalizeDeployer(ctx *deploy.Context) error {
	ctx.Info("Start to finalize Deployer")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeployerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudapiv1alpha1.Deployer{}).
		Complete(r)
}
