/*
SPDX-License-Identifier: CC0-1.0
https://creativecommons.org/publicdomain/zero/1.0/
*/
package util

import (
	"context"
	"fmt"

	k8s "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig :=
			clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

func GetPOD(ctx context.Context, ns, pod string) (*k8s.Pod, error) {
	clientset, err := GetClientset()
	if err != nil {
		return nil, err
	}
	listOptions := meta.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", pod),
	}
	api := clientset.CoreV1()
	pods, err := api.Pods(ns).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) != 1 {
		return nil, fmt.Errorf("Matching PODs %d", len(pods.Items))
	}
	return &pods.Items[0], nil
}
