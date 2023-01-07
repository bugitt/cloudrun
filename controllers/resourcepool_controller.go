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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudapiv1alpha1 "github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/types"
	"github.com/pkg/errors"
)

// ResourcePoolReconciler reconciles a ResourcePool object
type ResourcePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=resourcepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=resourcepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudapi.scs.buaa.edu.cn,resources=resourcepools/finalizers,verbs=update

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ResourcePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "resourcePool", req.NamespacedName)
	resourcePool := &cloudapiv1alpha1.ResourcePool{}
	err := r.Get(ctx, req.NamespacedName, resourcePool)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("ResourcePool resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ResourcePool.")
		return ctrl.Result{}, errors.Wrap(err, "failed to get ResourcePool")
	}

	// check validation of the resource pool
	used := &types.Resource{}
	for _, usage := range resourcePool.Spec.Usage {
		used.CPU += usage.Resource.CPU
		used.Memory += usage.Resource.Memory
	}
	if used.CPU+resourcePool.Spec.Free.CPU != resourcePool.Spec.Capacity.CPU {
		logger.Error(nil, "CPU usage is not equal to capacity.")
		return ctrl.Result{}, errors.New("CPU usage is not equal to capacity")
	}
	if used.Memory+resourcePool.Spec.Free.Memory != resourcePool.Spec.Capacity.Memory {
		logger.Error(nil, "Memory usage is not equal to capacity.")
		return ctrl.Result{}, errors.New("Memory usage is not equal to capacity")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourcePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudapiv1alpha1.ResourcePool{}).
		Complete(r)
}
