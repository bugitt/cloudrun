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

	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/cri-api/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/build"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/bugitt/cloudrun/controllers/finalize"
	"github.com/pkg/errors"
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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Builder object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *BuilderReconciler) Reconcile(originalCtx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(originalCtx)
	ctx := core.NewContext(originalCtx, r.Client, req)
	builder := &cloudapiv1alpha1.Builder{}
	err := r.Get(originalCtx, req.NamespacedName, builder)
	if err != nil {
		if kerrors.IsNotFound(err) {
			ctx.Info("Memcached resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		ctx.Error(err, "Failed to get Builder.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get builder CRD")
	}

	builder.Status.OtherMessage = "haha"
	_ = r.Update(originalCtx, builder)

	isToBeDeleted := builder.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		if finalize.Contains(builder) {
			if err := r.finalizeBuilder(ctx, builder); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to finalize builder CRD")
			}

			finalize.Remove(builder)
			err := r.Update(originalCtx, builder)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to update builder CRD")
			}
		}
		finalize.Add(builder)
		return ctrl.Result{}, nil
	}

	if status := builder.Status.Status; status == v1alpha1.StatusDone || status == v1alpha1.StatusFailed {
		ctx.Info("Builder job has been done or failed. Ignoring.")
		return ctrl.Result{}, nil
	}

	// create and exec image build job
	if err := build.Setup(ctx, builder); err != nil {
		ctx.Error(err, "Failed to setup builder job.")
		return ctrl.Result{}, errors.Wrap(err, "failed to setup builder job")
	}

	return ctrl.Result{}, nil
}

func (r *BuilderReconciler) finalizeBuilder(ctx core.Context, builder *v1alpha1.Builder) error {
	ctx.Info("Finalizing Memcached")
	return build.Delete(ctx, builder)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuilderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudapiv1alpha1.Builder{}).
		Complete(r)
}
