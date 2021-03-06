/*
Copyright 2018 The Knative Authors

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

// crd contains defintions of resource instances which are useful across integration tests

package test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/knative/pkg/test/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/build-pipeline/pkg/apis/pipeline/v1alpha1"
	tb "github.com/knative/build-pipeline/test/builder"
)

const (
	hwTaskName          = "helloworld"
	hwTaskRunName       = "helloworld-run"
	hwValidationPodName = "helloworld-validation-busybox"
	hwPipelineName      = "helloworld-pipeline"
	hwPipelineRunName   = "helloworld-pipelinerun"
	hwPipelineTaskName1 = "helloworld-task-1"
	hwPipelineTaskName2 = "helloworld-task-2"
	hwSecret            = "helloworld-secret"
	hwSA                = "helloworld-sa"

	logPath = "/logs"
	logFile = "process-log.txt"

	hwContainerName = "helloworld-busybox"
	taskOutput      = "do you want to build a snowman"
	buildOutput     = "Build successful"
)

func getHelloWorldValidationPod(namespace, volumeClaimName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      hwValidationPodName,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  hwValidationPodName,
				Image: "busybox",
				Command: []string{
					"cat",
				},
				Args: []string{fmt.Sprintf("%s/%s", logPath, logFile)},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "scratch",
						MountPath: logPath,
					},
				},
			}},
			Volumes: []corev1.Volume{{
				Name: "scratch",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: volumeClaimName,
					},
				},
			}},
		},
	}
}

func getHelloWorldTask(namespace string, args []string) *v1alpha1.Task {
	return tb.Task(hwTaskName, namespace,
		tb.TaskSpec(tb.Step(hwContainerName, "busybox", tb.Command(args...))),
	)
}

func getHelloWorldTaskRun(namespace string) *v1alpha1.TaskRun {
	return tb.TaskRun(hwTaskRunName, namespace, tb.TaskRunSpec(tb.TaskRunTaskRef(hwTaskName)))
}

func getBuildOutputFromVolume(t *testing.T, logger *logging.BaseLogger, c *clients, namespace, testStr string) string {
	t.Helper()
	// Create Validation Pod
	pods := c.KubeClient.Kube.CoreV1().Pods(namespace)

	// Volume created for Task should have the same name as the Task
	if _, err := pods.Create(getHelloWorldValidationPod(namespace, hwTaskRunName)); err != nil {
		t.Fatalf("failed to create Validation pod to mount volume `%s`: %s", hwTaskRunName, err)
	}

	logger.Infof("Waiting for pod with test volume %s to come up so we can read logs from it", hwTaskRunName)
	if err := WaitForPodState(c, hwValidationPodName, namespace, func(p *corev1.Pod) (bool, error) {
		// the "Running" status is used as "Succeeded" caused issues as the pod succeeds and restarts quickly
		// there might be a race condition here and possibly a better way of handling this, perhaps using a Job or different state validation
		if p.Status.Phase == corev1.PodRunning {
			return true, nil
		}
		return false, nil
	}, "ValidationPodCompleted"); err != nil {
		t.Fatalf("error waiting for Pod %s to finish: %s", hwValidationPodName, err)
	}

	// Get validation pod logs and verify that the build executed a container w/ desired output
	req := pods.GetLogs(hwValidationPodName, &corev1.PodLogOptions{})
	readCloser, err := req.Stream()
	if err != nil {
		t.Fatalf("failed to open stream to read: %v", err)
	}
	defer readCloser.Close()
	var buf bytes.Buffer
	out := bufio.NewWriter(&buf)
	_, err = io.Copy(out, readCloser)
	return buf.String()
}
