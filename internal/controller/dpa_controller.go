/*
Copyright 2025.

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

package controller

import (
	"context"
	"time"

	"fmt"
	autoscalerv1alpha1 "github.com/itsCheithanya/kubernetes-dynamic-pod-autoscaler/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const pollInterval = 10 * time.Second

// DPAReconciler reconciles a DPA object
type DPAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaler.itscheithanya.com,resources=dpas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.itscheithanya.com,resources=dpas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaler.itscheithanya.com,resources=dpas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DPA object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *DPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var dpa autoscalerv1alpha1.DPA
	if err := r.Get(ctx, req.NamespacedName, &dpa); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch target deployment
	var deploy appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: dpa.Spec.Namespace,
		Name:      dpa.Spec.DeploymentName,
	}, &deploy); err != nil {
		log.Error(err, "Failed to get target deployment")
		return ctrl.Result{}, err
	}
	scrapeURL := dpa.Spec.ScrapeURL

	//get the current rps from scrape url and the predicted rps from the model
	currentRPS, predictedRPS, err := Predicter(scrapeURL)
	if err != nil {
		log.Error(err, "AI prediction failed, skipping this reconcile cycle")
		return ctrl.Result{}, err
	}
	//print the currentRPS, predictedRPS
	fmt.Printf("currentRPS is: %f \n", currentRPS)
	fmt.Printf("predictedRPS is: %f \n", predictedRPS)
	//call the DPA algorithm
	scaled := r.DynamicPodAutoscaler(ctx, currentRPS, predictedRPS, dpa, &deploy)

	if scaled {
		now := metav1.Now()
		dpa.Status.LastScaleTime = &now
		dpa.Status.CurrentReplicas = *deploy.Spec.Replicas
		r.Status().Update(ctx, &dpa)
	}
	return ctrl.Result{RequeueAfter: pollInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalerv1alpha1.DPA{}).
		Named("dpa").
		Complete(r)
}
