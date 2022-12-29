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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/finalize"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
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

func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "workflow", req.NamespacedName)

	workflow := &cloudapiv1alpha1.Workflow{}

	err := r.Get(ctx, req.NamespacedName, workflow)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("Workflow resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workflow.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get workflow")
	}

	isToBeDeleted := workflow.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(workflow) {
			if err := r.finalizeWorkflow(ctx); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to finalize workflow")
			}

			finalize.Remove(workflow)
			err := r.Update(ctx, workflow)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update workflow")
			}
		}
		return ctrl.Result{}, nil
	}

	if !finalize.Contains(workflow) {
		finalize.Add(workflow)
		if err := r.Update(ctx, workflow); err != nil {
			logger.Error(err, "Failed to update Workflow after add finalizer")
			return ctrl.Result{}, errors.Wrap(err, "failed to update workflow")
		}
	}

	if workflow.Status.Base == nil {
		workflow.Status.Base = &types.CommonStatus{}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)
	}

	if workflow.Spec.Round == -1 {
		logger.Info("Builder is not ready. Ignoring.")
		workflow.Status.Base.Status = types.StatusUNDO
		return ctrl.Result{}, r.Status().Update(ctx, workflow)
	}

	if workflow.Status.Base.CurrentRound < workflow.Spec.Round {
		workflow.Status.Base.CurrentRound = workflow.Spec.Round
		workflow.Status.Base.Status = types.StatusPending
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)
	}

	status := workflow.Status.Base
	switch status.Status {

	case types.StatusUNDO:
		workflow.Status.Base.Status = types.StatusPending
		workflow.Status.Stage = cloudapiv1alpha1.WorkflowStagePending
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)

	case types.StatusPending:
		workflow.Status.Base.Status = types.StatusDoing
		workflow.Status.Stage = cloudapiv1alpha1.WorkflowStagePending
		return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)

	case types.StatusDoing:
		switch workflow.Status.Stage {
		case cloudapiv1alpha1.WorkflowStagePending:
			workflow.Status.Stage = cloudapiv1alpha1.WorkflowStageBuilding
			// setup all the builders
			for _, builderName := range workflow.Spec.BuilderList {
				builder := &cloudapiv1alpha1.Builder{}
				err := r.Get(ctx, client.ObjectKey{Namespace: builderName.Namespace, Name: builderName.Name}, builder)
				if err != nil {
					logger.Error(err, "Failed to get Builder.")
					return ctrl.Result{}, errors.Wrap(err, "failed to get builder")
				}
				builder.Spec.Round = builder.Status.Base.CurrentRound + 1
				if err := r.Update(ctx, builder); err != nil {
					logger.Error(err, "Failed to setup Builder.")
					return ctrl.Result{}, errors.Wrap(err, "failed to setup builder")
				}
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)

		case cloudapiv1alpha1.WorkflowStageBuilding:
			failed, done := false, true
			for _, builderName := range workflow.Spec.BuilderList {
				builder := &cloudapiv1alpha1.Builder{}
				err := r.Get(ctx, client.ObjectKey{Namespace: builderName.Namespace, Name: builderName.Name}, builder)
				if err != nil {
					logger.Error(err, "Failed to get Builder.")
					return ctrl.Result{}, errors.Wrap(err, "failed to get builder")
				}
				builderStatus := builder.Status.Base.Status
				if builderStatus == types.StatusFailed {
					failed = true
					break
				}
				if builderStatus != types.StatusDone {
					done = false
				}
			}
			if failed {
				workflow.Status.Base.Status = types.StatusFailed
				return ctrl.Result{}, r.Status().Update(ctx, workflow)
			}
			if done {
				workflow.Status.Stage = cloudapiv1alpha1.WorkflowStageDeploying
				// setup all the deployers
				for _, deployerName := range workflow.Spec.DeployerList {
					deployer := &cloudapiv1alpha1.Deployer{}
					err := r.Get(ctx, client.ObjectKey{Namespace: deployerName.Namespace, Name: deployerName.Name}, deployer)
					if err != nil {
						logger.Error(err, "Failed to get Deployer.")
						return ctrl.Result{}, errors.Wrap(err, "failed to get deployer")
					}
					deployer.Spec.Round = deployer.Status.Base.CurrentRound + 1
					if err := r.Update(ctx, deployer); err != nil {
						logger.Error(err, "Failed to setup Deployer.")
						return ctrl.Result{}, errors.Wrap(err, "failed to setup deployer")
					}
				}
				return ctrl.Result{RequeueAfter: 1 * time.Second}, r.Status().Update(ctx, workflow)
			}

		case cloudapiv1alpha1.WorkflowStageDeploying:
			failed, done := false, true
			for _, deployerName := range workflow.Spec.DeployerList {
				deployer := &cloudapiv1alpha1.Deployer{}
				err := r.Get(ctx, client.ObjectKey{Namespace: deployerName.Namespace, Name: deployerName.Name}, deployer)
				if err != nil {
					logger.Error(err, "Failed to get Deployer.")
					return ctrl.Result{}, errors.Wrap(err, "failed to get deployer")
				}
				deployerStatus := deployer.Status.Base.Status
				if deployerStatus == types.StatusFailed {
					failed = true
					break
				}
				if deployerStatus != types.StatusDone {
					done = false
				}
			}
			if failed {
				workflow.Status.Base.Status = types.StatusFailed
				return ctrl.Result{}, r.Status().Update(ctx, workflow)
			}
			if done {
				workflow.Status.Stage = cloudapiv1alpha1.WorkflowStageServing
				workflow.Status.Base.Status = types.StatusDone
				return ctrl.Result{}, r.Status().Update(ctx, workflow)
			}
		}

	case types.StatusDone, types.StatusFailed:
		logger.Info(fmt.Sprintf("Workflow is done. Ignoring. Status: %s", status.Status))
		return ctrl.Result{}, nil
	}

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
