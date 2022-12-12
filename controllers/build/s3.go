package build

import (
	"bytes"
	"path/filepath"
	"text/template"

	"github.com/bugitt/cloudrun/api/v1alpha1"
	"github.com/bugitt/cloudrun/controllers/core"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const mcConfigTemplate = `
{
	"version": "10",
	"aliases": {
		"{{ .AliasName }}": {
			"url": "{{ .URL }}",
			"accessKey": "{{ .AccessKey }}",
			"secretKey": "{{ .SecretKey }}",
			"api": "s3v4",
			"path": "auto"
		}
	}
}
`

const (
	mcAliasName       = "scs-internal"
	mcConfigDir       = "/mc-config"
	mcConfigFilename  = "config.json"
	s3DefaultEndpoint = "s3.amazonaws.com"
)

type _McConfig struct {
	AliasName string
	URL       string
	AccessKey string
	SecretKey string
}

// mcConfigJson renders mc config template
func mcConfigJson(s3Opt v1alpha1.BuilderContextS3) string {
	tpl := template.Must(template.New("mcConfig").Parse(mcConfigTemplate))
	if s3Opt.Endpoint == "" {
		s3Opt.Endpoint = s3DefaultEndpoint
	}
	if s3Opt.Scheme == "" {
		s3Opt.Scheme = "https"
	}
	url := s3Opt.Scheme + "://" + s3Opt.Endpoint

	mcConfig := _McConfig{
		AliasName: mcAliasName,
		URL:       url,
		AccessKey: s3Opt.AccessKeyID,
		SecretKey: s3Opt.SecretAccessKey,
	}

	var tplResult bytes.Buffer
	_ = tpl.Execute(&tplResult, mcConfig)
	return tplResult.String()
}

func copyObjectFromS3Args(objectKey string) []string {
	return []string{"cp", "--mc-config-dir", mcConfigDir, filepath.Join(mcAliasName, objectKey), contextTarFileFullPath}
}

// addS3InitContainers adds needed containers and volumes to fetch context from s3 to the podTemplateSpec
// this container will copy context from s3 to local emptyDir,
// and the context file will be named to {{ prepareContextDir }}/{{ contextTarName }} (defined in build/builder.go)
func addS3InitContainers(ctx core.Context, s3ContextCfg *v1alpha1.BuilderContextS3, pod *apiv1.PodSpec) error {
	// create configMap to store the mc config json
	mcConfigMap := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ctx.Name() + "-mc-config-json",
			Namespace: ctx.Namespace(),
		},
		Data: map[string]string{
			mcConfigFilename: mcConfigJson(*s3ContextCfg),
		},
	}
	if err := ctx.CreateResource(mcConfigMap, true); err != nil {
		return err
	}

	mcConfigVolume := apiv1.Volume{
		Name: "mc-config",
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: mcConfigMap.Name,
				},
				Items: []apiv1.KeyToPath{
					{
						Key:  mcConfigFilename,
						Path: mcConfigFilename,
					},
				},
			},
		},
	}
	mcConfigVolumeMount := apiv1.VolumeMount{
		Name:      mcConfigVolume.Name,
		MountPath: mcConfigDir,
	}

	// we also need an emptyDir to store the context tar file
	prepareContextTarVolume := apiv1.Volume{
		Name: prepareContextTarVolumeName,
		VolumeSource: apiv1.VolumeSource{
			EmptyDir: &apiv1.EmptyDirVolumeSource{},
		},
	}
	emptyDirVolumeMount := apiv1.VolumeMount{
		Name:      prepareContextTarVolume.Name,
		MountPath: prepareContextTarVolumeMountPath,
	}

	pod.Volumes = append(pod.Volumes, mcConfigVolume, prepareContextTarVolume)

	mcCopyContainer := apiv1.Container{
		Name:         "fetch-s3-context",
		Image:        minioImageName,
		Args:         copyObjectFromS3Args(s3ContextCfg.ObjectKey),
		VolumeMounts: []apiv1.VolumeMount{mcConfigVolumeMount, emptyDirVolumeMount},
	}

	// we need a container to unzip the context tar file
	unzipContainer := apiv1.Container{
		Name:         "unzip-context",
		Image:        unzipImageName,
		Command:      []string{"sh", "-c"},
		Args:         []string{"unarr", contextTarFileFullPath, workspaceVolumeMountPath},
		VolumeMounts: []apiv1.VolumeMount{emptyDirVolumeMount, workspaceMount()},
	}

	pod.InitContainers = append(pod.InitContainers, mcCopyContainer, unzipContainer)

	return nil
}
