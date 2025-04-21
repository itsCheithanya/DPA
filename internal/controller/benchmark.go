package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	autoscalerv1alpha1 "github.com/itsCheithanya/kubernetes-dynamic-pod-autoscaler/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	sigs.k8s.io/controller-runtime/pkg/client"
)

type BenchmarkResult struct {
	TotalCPUUsageMilli   float64
	TotalMemoryUsageMiB float64
	Requests             int
}

func BenchmarkAndCalibrateFromDPA(
	ctx context.Context,
	metricsClientset *metricsclient.Clientset,
	k8sClient client.Client,
	deploy *appsv1.Deployment,
	dpa autoscalerv1alpha1.DPA,
	requestsPerSec int,
	duration time.Duration,
) (float64, float64, error) {
	var totalCPU, totalMemory float64
	var totalRequests int

	for _, methodMap := range dpa.Spec.BenchmarkURls {
		for method, requests := range methodMap {
			for reqName, spec := range requests {
				var target vegeta.Target
				target.Method = strings.ToUpper(method)
				target.URL = spec.URL

				if target.Method == "POST" && len(spec.Payload) > 0 {
					payloadBytes, err := json.Marshal(spec.Payload)
					if err != nil {
						return 0, 0, fmt.Errorf("failed to marshal payload: %w", err)
					}
					target.Body = payloadBytes
				}
				if target.Method == "GET" && spec.Params != "" {
					target.URL += spec.Params
				}

				targeter := vegeta.NewStaticTargeter(target)
				rat := vegeta.Rate{Freq: requestsPerSec, Per: time.Second}
				attacker := vegeta.NewAttacker()

				fmt.Printf("Running benchmark for %s %s...\n", target.Method, target.URL)
				res := attacker.Attack(targeter, rat, duration, fmt.Sprintf("Benchmark-%s", reqName))

				for r := range res {
					if r.Error != "" {
						fmt.Printf("Request error: %s\n", r.Error)
						continue
					}
					totalRequests++
				}
			}
		}
	}

	// After all requests, wait a few seconds to allow metrics to stabilize
	time.Sleep(5 * time.Second)

	// Use your custom implementation to compute average resource utilization
	cpuPerReq, memPerReq, err := getAverageUtilization(ctx, metricsClientset, deploy, totalRequests)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get average utilization: %w", err)
	}

	return cpuPerReq, memPerReq, nil
}

func getAverageUtilization(
	ctx context.Context,
	metricsClientset *metricsclient.Clientset,
	deploy *appsv1.Deployment,
	totalRequests int,
) (float64, float64, error) {

	podMetricsList, err := metricsClientset.MetricsV1beta1().PodMetricses(deploy.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deploy.Labels["app"]),
	})


	if err != nil {
		return 0, 0, err
	}

	var totalCPU, totalMem float64
	for _, pod := range podMetricsList.Items {
		for _, c := range pod.Containers {
			cpu := c.Usage.Cpu().MilliValue()
			mem := c.Usage.Memory().Value() / (1024 * 1024)
			totalCPU += float64(cpu)
			totalMem += float64(mem)
		}
	}

	cpuPerReq := totalCPU / float64(totalRequests)
	memPerReq := totalMem / float64(totalRequests)
	return cpuPerReq, memPerReq, nil
}
