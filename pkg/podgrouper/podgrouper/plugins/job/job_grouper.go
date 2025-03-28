// Copyright 2025 NVIDIA CORPORATION
// SPDX-License-Identifier: Apache-2.0

package job

import (
	"context"
	"fmt"

	"github.com/NVIDIA/KAI-scheduler/pkg/apis/scheduling/v2alpha2"
	"github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgroup"
	defaultpodgrouper "github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgrouper/plugins"
	"github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgrouper/plugins/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type K8sJobGrouper struct {
	client                   client.Client
	searchForLegacyPodGroups bool
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs/finalizers,verbs=patch;update;create

var logger = log.FromContext(context.Background())

func NewK8sJobGrouper(client client.Client, searchForLegacyPodGroups bool) *K8sJobGrouper {
	return &K8sJobGrouper{client: client, searchForLegacyPodGroups: searchForLegacyPodGroups}
}

func (g *K8sJobGrouper) GetPodGroupMetadata(topOwner *unstructured.Unstructured, pod *v1.Pod, _ ...*metav1.PartialObjectMetadata) (*podgroup.Metadata, error) {
	podGroupMetadata, err := defaultpodgrouper.GetPodGroupMetadata(topOwner, pod)
	if err != nil {
		return nil, err
	}

	podGroupMetadata.Name, err = g.calcPodGroupName(topOwner, pod)
	if err != nil {
		return nil, err
	}

	return podGroupMetadata, nil
}

func (g *K8sJobGrouper) calcPodGroupName(topOwner *unstructured.Unstructured, pod *v1.Pod) (string, error) {
	if g.searchForLegacyPodGroups {
		legacyName := calcLegacyName(topOwner, pod)
		if legacyName != "" {
			legacyPodGroupObj := &v2alpha2.PodGroup{}
			err := g.client.Get(context.Background(), types.NamespacedName{Namespace: pod.Namespace, Name: legacyName},
				legacyPodGroupObj)
			if err == nil {
				logger.V(1).Info("Using legacy pod-group %s/%s", pod.Namespace, legacyName)
				return legacyName, nil
			} else if !errors.IsNotFound(err) {
				logger.V(1).Error(err,
					"While searching for legacy pod group for pod %s/%s, an error has occurred.",
					pod.Namespace, legacyName)
				return "", err
			}
		}
	}

	return fmt.Sprintf("%s-%s-%s", constants.PodGroupNamePrefix, pod.Name, topOwner.GetUID()), nil
}

func calcLegacyName(topOwner *unstructured.Unstructured, pod *v1.Pod) string {
	jobParallelism, found, err := unstructured.NestedInt64(topOwner.Object, "spec", "parallelism")

	var baseName string
	if found && err == nil && jobParallelism > 1 {
		baseName = pod.Name
	} else {
		baseName = topOwner.GetName()
	}

	return fmt.Sprintf("%s-%s-%s", constants.PodGroupNamePrefix, baseName, topOwner.GetUID())
}
