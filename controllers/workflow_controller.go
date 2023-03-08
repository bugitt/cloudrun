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

	"github.com/bugitt/cloudrun/api/v1alpha1"
	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/controllers/finalize"
	workflowcore "github.com/bugitt/cloudrun/controllers/workflow"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
)

// WorkflowReconciler reconciles a Workflow object
type WorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=workflows,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=workflows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=workflows/finalizers,verbs=update
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=builders/finalizers,verbs=update
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=deployers/finalizers,verbs=update

func (r *WorkflowReconciler) Reconcile(originalCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(originalCtx, "workflow", req.NamespacedName)

	workflow := &cloudapiv1alpha1.Workflow{}

	err := r.Get(originalCtx, req.NamespacedName, workflow)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("Workflow resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workflow.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get workflow")
	}

	ctx := workflowcore.NewContext(
		originalCtx,
		r.Client,
		logger,
		workflow,
	)

	defer func() {
		if re := recover(); re != nil {
			logger.Error(re.(error), "Reconcile panic")
			core.PublishStatus(ctx, workflow, re.(error))
		}
	}()

	isToBeDeleted := workflow.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(workflow) {
			if err := r.finalizeWorkflow(originalCtx); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to finalize workflow")
			}

			finalize.Remove(workflow)
			err := r.Update(originalCtx, workflow)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update workflow")
			}
		}
		return ctrl.Result{}, nil
	}

	if !finalize.Contains(workflow) {
		finalize.Add(workflow)
		if err := r.Update(originalCtx, workflow); err != nil {
			logger.Error(err, "Failed to update Workflow after add finalizer")
			return ctrl.Result{}, errors.Wrap(err, "failed to update workflow")
		}
	}

	if workflow.Status.Base == nil {
		workflow.Status.Base = &types.CommonStatus{}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(originalCtx, workflow)
	}

	if workflow.Spec.Round < workflow.CommonStatus().CurrentRound {
		logger.Info("Builder is not ready. Ignoring.")
		if err := ctx.GenerateWorkload(); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to generate workload")
		}
		workflow.Status.Base.Status = types.StatusUNDO
		workflow.Status.Stage = v1alpha1.WorkflowStagePending
		return ctrl.Result{}, r.Status().Update(originalCtx, workflow)
	}

	if workflow.Spec.Round > workflow.CommonStatus().CurrentRound {
		if err := core.BackupState(workflow, workflow.Spec); err != nil {
			ctx.Error(err, "Failed to backup state")
			return ctrl.Result{}, errors.Wrap(err, "failed to backup state")
		}
		if err := ctx.GenerateWorkload(); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to generate workload")
		}
		workflow.Status.Stage = v1alpha1.WorkflowStagePending
		workflow.CommonStatus().Status = types.StatusPending
		workflow.CommonStatus().CurrentRound = workflow.Spec.Round
		workflow.CommonStatus().StartTime = time.Now().Unix()
		workflow.CommonStatus().EndTime = 0

		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(originalCtx, workflow)
	}

	builder := &cloudapiv1alpha1.Builder{}
	if err := ctx.Get(ctx, ktypes.NamespacedName{Name: ctx.Name(), Namespace: ctx.Namespace()}, builder); err != nil {
		if client.IgnoreNotFound(err) == nil {
			builder = nil
		} else {
			return ctrl.Result{}, errors.Wrap(err, "failed to get builder")
		}
	}

	deployer := &cloudapiv1alpha1.Deployer{}
	if err := ctx.Get(ctx, ktypes.NamespacedName{Name: ctx.Name(), Namespace: ctx.Namespace()}, deployer); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to get deployer")
	}

	if builder == nil {
		workflow.CommonStatus().Status = deployer.CommonStatus().Status
	} else {
		if builder.CommonStatus().Status == types.StatusDoing {
			workflow.CommonStatus().Status = types.StatusDoing
		} else if builder.CommonStatus().HashDone() {
			workflow.CommonStatus().Status = deployer.CommonStatus().Status
			if !workflow.CommonStatus().HasStarted() {
				workflow.CommonStatus().Status = types.StatusDoing
			}
		}
	}

	core.PublishStatus(ctx, workflow, nil)

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *WorkflowReconciler) finalizeWorkflow(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Finalizing workflow")

	// do nothing

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudapiv1alpha1.Workflow{}).
		Complete(r)
}
