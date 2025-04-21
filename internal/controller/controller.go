package controller

import (
	"context"
	// "encoding/json"
	"fmt"
	"math"
	// "io/ioutil"
	// "net/http"
	// "strconv"
	// "time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// "sigs.k8s.io/controller-runtime/pkg/client/config"// I added this
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	autoscalerv1alpha1 "github.com/itsCheithanya/kubernetes-dynamic-pod-autoscaler/api/v1alpha1"
)

const podsMinimum = 1
const RRS = 0.5 // reduction ratio for scaling down

func (r *DPAReconciler) DynamicPodAutoscaler(ctx context.Context, currentRPS float64, predictedRPS float64, dpa autoscalerv1alpha1.DPA, deploy *appsv1.Deployment) bool {
	log := log.FromContext(ctx)
	currentWorkload := int32(math.Ceil(currentRPS))
	predictedWorkload := int32(math.Ceil(predictedRPS))

	// cfg, err := config.GetConfig()
	// if err != nil {
	// 	log.Error(err, "unable to get kubeconfig or in cluster config")
	// 	os.Exit(1)
	// }
	cfg := ctrl.GetConfigOrDie()
	// use the cfg from the manager
	metricsClientset, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "Failed to get metrics client")
		return false
	}
	//benchmark result
	cpuUtilizationPerReq, memoryUtilizationPerReq:= 0.01000,0.200000
	// cpuUtilizationPerReq, memoryUtilizationPerReq, err := r.getAverageResourceUtilization(ctx, metricsClientset, deploy)
	if err != nil {
		log.Error(err, "Failed to calculate resource utilization")
		return false
	}
	cpuLimit, memoryLimit, err := getResourceLimits(deploy)
	fmt.Printf("The deployment has %f cpuLimit and %f memLimit set: \n", cpuLimit, memoryLimit)

	//compute the bottleneck resource
	var maxWorkloadPerPod int32 //in rps
	var bottleneck string
	if (cpuLimit - cpuUtilizationPerReq) < (memoryLimit - memoryUtilizationPerReq) {
		bottleneck = "cpu"
		// Compute max workload per pod
		if cpuUtilizationPerReq == 0 {
			fmt.Printf("%s utilization is 0, skipping scaling decision \n", bottleneck)
		}
		maxWorkloadPerPod = int32((math.Ceil(cpuLimit) * float64(currentWorkload)) / math.Ceil(cpuUtilizationPerReq))
		fmt.Printf("max workload per pod cpu: %d\n", maxWorkloadPerPod)

	} else {
		bottleneck = "memory"
		// Compute max workload per pod
		if memoryUtilizationPerReq == 0 {
			fmt.Printf("%s utilization is 0, skipping scaling decision \n", bottleneck)
		}

		maxWorkloadPerPod = int32((math.Ceil(memoryLimit) * float64(currentWorkload)) / math.Ceil(memoryUtilizationPerReq))
		fmt.Printf("max workload per pod mem: %d \n", maxWorkloadPerPod)

	}
	fmt.Printf("predictedWorkload in rps %d \n", predictedWorkload)
	fmt.Printf("currentWorkload in rps %d \n", currentWorkload)
	var predictedFuturePods int32
	if maxWorkloadPerPod != 0 {
		predictedFuturePods = int32(predictedWorkload / maxWorkloadPerPod)
	} else {
		predictedFuturePods = 1
	}
	currentPods := *deploy.Spec.Replicas

	fmt.Printf("The current number of pods: %d \n", currentPods)
	fmt.Printf("scaling decision take to have: %d  of pods\n", predictedFuturePods)

	if predictedFuturePods > currentPods {
		// *deploy.Spec.Replicas = predictedFuturePods
		// r.Update(ctx, deploy)
		fmt.Printf("Current number of pods: %d \n", currentPods)
		fmt.Printf("scaling up to: %d \n", predictedFuturePods)
		return true
	} else if predictedFuturePods < currentPods {
		surplus := int32(float64(currentPods-predictedFuturePods) * RRS)
		adjustedPods := currentPods - surplus
		if adjustedPods < podsMinimum {
			adjustedPods = podsMinimum
		}
		// *deploy.Spec.Replicas = adjustedPods
		// r.Update(ctx, deploy)
		fmt.Printf("Current number of pods: %d \n", currentPods)
		fmt.Printf("scaling up to: %d \n", predictedFuturePods)
		return true
	} else {
		fmt.Printf("No scaling done \n")
		return false
	}

}

func (r *DPAReconciler) getAverageResourceUtilization(ctx context.Context, metricsClientset metricsclient.Interface, deploy *appsv1.Deployment) (float64, float64, error) {

	// List pods belonging to the deployment
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(deploy.Namespace),
		client.MatchingLabels(deploy.Spec.Selector.MatchLabels),
	}
	if err := r.List(ctx, podList, listOpts...); err != nil {
		return 0.0, 0.0, fmt.Errorf("listing pods failed: %w", err)
	}

	var totalCPUUsage, totalMemUsage float64
	var containerCount int

	for _, pod := range podList.Items {
		podMetrics, err := metricsClientset.MetricsV1beta1().PodMetricses(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			// Skip this pod and log the error if metrics are missing
			fmt.Printf("Warning: failed to get metrics for pod %s: %v\n", pod.Name, err)
			continue
		}

		for _, container := range podMetrics.Containers {
			cpuQuantity := container.Usage[corev1.ResourceCPU]
			memQuantity := container.Usage[corev1.ResourceMemory]

			cpuMilli := float64(cpuQuantity.MilliValue())          // in millicores
			memMiB := float64(memQuantity.Value()) / (1024 * 1024) // bytes to MiB

			totalCPUUsage += cpuMilli
			totalMemUsage += memMiB
			containerCount++
		}
	}

	if containerCount == 0 {
		return 0.0, 0.0, fmt.Errorf("no container metrics available")
	}

	avgCPU := totalCPUUsage / float64(containerCount)
	avgMem := totalMemUsage / float64(containerCount)

	// Return the bottleneck (max usage) between CPU and memory
	// return math.Max(avgCPU, avgMem), nil

	//we return both and check in the main aglorithm which is the bootleneck resource
	fmt.Printf("The deployment %s has %d pod/pods with %f avgCPU and %f avgMem utilization: \n", deploy.Name, len(podList.Items), avgCPU, avgMem)
	return avgCPU, avgMem, nil
}

// TODO:
// The averaging logic be necessar only if:
// Your deployment has multiple containers with different limits, or
// You want to take an average across heterogeneous container setups (which is rare in well-structured deployments)
func getResourceLimits(deploy *appsv1.Deployment) (float64, float64, error) {
	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return 0.0, 0.0, fmt.Errorf("No containers found for this deployment")
	}

	container := deploy.Spec.Template.Spec.Containers[0]

	var cpuLimit float64
	var memLimit float64

	if limit := container.Resources.Limits.Cpu(); limit != nil {
		cpuLimit = float64(limit.MilliValue()) // in millicores
	} else {
		return 0.0, 0.0, fmt.Errorf("CPU limits is not decalred for the deployment: %s", deploy.ObjectMeta.Name)
	}
	if limit := container.Resources.Limits.Memory(); limit != nil {
		memLimit = float64(limit.Value()) / (1024 * 1024) // in MiB
	} else {
		return 0.0, 0.0, fmt.Errorf("Memory limits is not decalred for the deployment: %s", deploy.ObjectMeta.Name)
	}

	// Return the bottleneck limit (whichever is lower)
	// return math.Min(cpuLimit, memLimit),nil

	//we return both and check in the main aglorithm which is the bootleneck resource
	return cpuLimit, memLimit, nil
}
