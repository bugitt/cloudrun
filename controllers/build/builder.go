package build

import (
	"path/filepath"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	contextTarName = "contextTar"
)

const (
	prepareContextTarVolumeName = "prepare-context-tar"
	workspaceVolumeName         = "workspace"

	prepareContextTarVolumeMountPath = "/prepare-context"
	workspaceVolumeMountPath         = "/workspace"
	pushSecretVolumeMountPath        = "/kaniko/.docker/"
)

const (
	minioImageName  = "minio/mc:RELEASE.2020-04-22T01-19-57Z"
	unzipImageName  = "loheagn/go-unarr:0.1.6"
	kanikoImageName = "scs.buaa.edu.cn:8081/iobs/kaniko-executor"
)

const (
	defaultPushSecretName = "push-secret"
)

var (
	contextTarFileFullPath = filepath.Join(prepareContextTarVolumeMountPath, contextTarName)
)

func workspaceMount() apiv1.VolumeMount {
	return apiv1.VolumeMount{
		Name:      workspaceVolumeName,
		MountPath: workspaceVolumeMountPath,
	}
}

func newJob(ctx core.Context, builder *v1alpha1.Builder) (*batchv1.Job, error) {
	// workspaceVolume is an emptyDir to store the context dir
	workspaceVolume := apiv1.Volume{
		Name: workspaceVolumeName,
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{},
		},
	}

	pushSecretName := builder.Spec.PushSecretName
	if pushSecretName == "" {
		pushSecretName = defaultPushSecretName
	}
	pushSecretVolume := apiv1.Volume{
		Name: "push-secret",
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: pushSecretName,
				Items: []apiv1.KeyToPath{
					{
						Key:  ".dockerconfigjson",
						Path: "config.json",
					},
				},
			},
		},
	}
	pushSecretVolumeMount := apiv1.VolumeMount{
		Name:      pushSecretVolume.Name,
		MountPath: pushSecretVolumeMountPath,
	}

	podSpec := apiv1.PodSpec{}

	mainContainer := apiv1.Container{
		Name:         "main",
		Image:        kanikoImageName,
		Args:         []string{"--dockerfile", filepath.Join(workspaceVolumeMountPath, builder.Spec.DockerfilePath), "--context", "dir:///workspace", "--destination", builder.Spec.Destination},
		VolumeMounts: []apiv1.VolumeMount{workspaceMount(), pushSecretVolumeMount},
	}

	podSpec.Volumes = []apiv1.Volume{workspaceVolume, pushSecretVolume}
	podSpec.Containers = []apiv1.Container{mainContainer}
	podSpec.RestartPolicy = "Never"

	switch {
	case builder.Spec.Context.S3 != nil:
		if err := addS3InitContainers(ctx, builder.Spec.Context.S3, &podSpec); err != nil {
			return nil, errors.Wrap(err, "add s3 init containers")
		}

		// TODO other cases
	}
	var backoffLimit int32 = 4
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      builder.Name,
			Namespace: builder.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      builder.Name,
					Namespace: builder.Namespace,
				},
				Spec: podSpec,
			},
			BackoffLimit: &backoffLimit,
		},
	}
	return job, nil
}

func cleanup(ctx core.Context) error {
	return ctx.CleanupResources()
}

func Setup(ctx core.Context, builder *v1alpha1.Builder) error {
	var job batchv1.Job
	// 1. check if the job is already running
	exist, err := ctx.GetResource(&job)
	if err != nil {
		return err
	}
	// 2. if not, create a new job
	if !exist {
		builder.Status.Status = v1alpha1.StatusPending
		if err := ctx.Update(ctx, builder); err != nil {
			return err
		}
		job, err := newJob(ctx, builder)
		if err != nil {
			return errors.Wrap(err, "create new job")
		}
		if err := ctx.CreateResource(job, false); err != nil {
			return err
		}
	}
	// 4. watch the job status
	if _, err := ctx.GetResource(&job); err != nil {
		return err
	}
	active, succeeded, failed := job.Status.Active, job.Status.Succeeded, job.Status.Failed
	switch {
	case succeeded > 0:
		{
			// 5. update the builder status
			builder.Status.Status = v1alpha1.StatusDone
			if err := ctx.Update(ctx, builder); err != nil {
				return err
			}
			// 6. if the job is success, delete the job
			return cleanup(ctx)
		}
	case failed > 0:
		{
			// 5. update the builder status
			builder.Status.Status = v1alpha1.StatusFailed
			if err := ctx.Update(ctx, builder); err != nil {
				return err
			}
			// 6. if the job is failed, create a new job
			return cleanup(ctx)
		}
	case active > 0:
		{
			// 5. update the builder status
			builder.Status.Status = v1alpha1.StatusDoing
			return ctx.Update(ctx, builder)
		}
	default:
		builder.Status.Status = v1alpha1.StatusUnknown
		return ctx.Update(ctx, builder)
	}
}

func Delete(ctx core.Context, builder *v1alpha1.Builder) error {
	return cleanup(ctx)
}
