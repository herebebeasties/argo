package k8sapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/argoproj/argo/v3/errors"
	"github.com/argoproj/argo/v3/workflow/common"
	execcommon "github.com/argoproj/argo/v3/workflow/executor/common"
)

type k8sAPIClient struct {
	clientset *kubernetes.Clientset
	config    *restclient.Config
	podName   string
	namespace string
}

var _ execcommon.KubernetesClientInterface = &k8sAPIClient{}

func newK8sAPIClient(clientset *kubernetes.Clientset, config *restclient.Config, podName, namespace string) (*k8sAPIClient, error) {
	return &k8sAPIClient{
		clientset: clientset,
		config:    config,
		podName:   podName,
		namespace: namespace,
	}, nil
}

func (c *k8sAPIClient) CreateArchive(ctx context.Context, containerID, sourcePath string) (*bytes.Buffer, error) {
	_, containerStatus, err := c.GetContainerStatus(ctx, containerID)
	if err != nil {
		return nil, err
	}
	command := []string{"tar", "cf", "-", sourcePath}
	exec, err := common.ExecPodContainer(c.config, c.namespace, c.podName, containerStatus.Name, true, false, command...)
	if err != nil {
		return nil, err
	}
	stdOut, _, err := common.GetExecutorOutput(exec)
	if err != nil {
		return nil, err
	}
	return stdOut, nil
}

func (c *k8sAPIClient) getLogsAsStream(ctx context.Context, containerID string) (io.ReadCloser, error) {
	_, containerStatus, err := c.GetContainerStatus(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return c.clientset.CoreV1().Pods(c.namespace).
		GetLogs(c.podName, &corev1.PodLogOptions{Container: containerStatus.Name, SinceTime: &metav1.Time{}}).Stream(ctx)
}

func (c *k8sAPIClient) getPod(ctx context.Context) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(c.namespace).Get(ctx, c.podName, metav1.GetOptions{})
}

func (c *k8sAPIClient) GetContainerStatus(ctx context.Context, containerID string) (*corev1.Pod, *corev1.ContainerStatus, error) {
	pod, err := c.getPod(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if execcommon.GetContainerID(&containerStatus) != containerID {
			continue
		}
		return pod, &containerStatus, nil
	}
	return nil, nil, errors.New(errors.CodeNotFound, fmt.Sprintf("containerID %q is not found in the pod %s", containerID, c.podName))
}

func (c *k8sAPIClient) KillContainer(pod *corev1.Pod, container *corev1.ContainerStatus, sig syscall.Signal) error {
	command := []string{"/bin/sh", "-c", fmt.Sprintf("kill -%d 1", sig)}
	exec, err := common.ExecPodContainer(c.config, c.namespace, c.podName, container.Name, false, true, command...)
	if err != nil {
		return err
	}
	_, _, err = common.GetExecutorOutput(exec)
	return err
}

func (c *k8sAPIClient) killGracefully(ctx context.Context, containerID string) error {
	return execcommon.KillGracefully(ctx, c, containerID)
}
