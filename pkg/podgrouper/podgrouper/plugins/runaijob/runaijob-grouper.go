// Copyright 2025 NVIDIA CORPORATION
// SPDX-License-Identifier: Apache-2.0

package runaijob

import (
	"context"
	"fmt"
	"strings"

	"github.com/NVIDIA/KAI-scheduler/pkg/apis/scheduling/v2alpha2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgroup"
	defaultpodgrouper "github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgrouper/plugins"
	"github.com/NVIDIA/KAI-scheduler/pkg/podgrouper/podgrouper/plugins/constants"
	"k8s.io/apimachinery/pkg/api/errors"
)

type RunaiJobGrouper struct {
	client                   client.Client
	searchForLegacyPodGroups bool
}

// +kubebuilder:rbac:groups=run.ai,resources=runaijobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=run.ai,resources=runaijobs/finalizers,verbs=patch;update;create

var logger = log.FromContext(context.Background())

func NewRunaiJobGrouper(client client.Client, searchForLegacyPodGroups bool) *RunaiJobGrouper {
	return &RunaiJobGrouper{client: client, searchForLegacyPodGroups: searchForLegacyPodGroups}
}

func (g *RunaiJobGrouper) GetPodGroupMetadata(topOwner *unstructured.Unstructured, pod *v1.Pod, _ ...*metav1.PartialObjectMetadata) (*podgroup.Metadata, error) {
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

func (g *RunaiJobGrouper) calcPodGroupName(topOwner *unstructured.Unstructured, pod *v1.Pod) (string, error) {
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

	pgName := pod.Name
	lastDashIndex := strings.LastIndex(pgName, "-")
	if lastDashIndex != -1 {
		pgName = pgName[:lastDashIndex]
	}

	return fmt.Sprintf("%s-%s-%s", constants.PodGroupNamePrefix, pgName, topOwner.GetUID()), nil
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
