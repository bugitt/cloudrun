package finalize

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const cloudRunFinalizer = "finalizer.cloudrun.scs.buaa.edu.cn"

func Add(obj client.Object) {
	_ = controllerutil.AddFinalizer(obj, cloudRunFinalizer)
}

func Contains(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, cloudRunFinalizer)
}

func Remove(obj client.Object) {
	_ = controllerutil.RemoveFinalizer(obj, cloudRunFinalizer)
}
