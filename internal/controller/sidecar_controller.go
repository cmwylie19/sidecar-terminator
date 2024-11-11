/*
Copyright 2024.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	terminatorv1alpha1 "github.com/cmwylie19/sidecar-terminator/api/v1alpha1"
)

// SidecarReconciler reconciles a Sidecar object
type SidecarReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=terminator.defenseunicorns.com,resources=sidecars,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=terminator.defenseunicorns.com,resources=sidecars/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=terminator.defenseunicorns.com,resources=sidecars/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get;list;watch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sidecar object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *SidecarReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Sidecar resource")

	sidecar := &terminatorv1alpha1.Sidecar{}
	if err := r.Get(ctx, req.NamespacedName, sidecar); err != nil {
		logger.Error(err, "Failed to get Sidecar resource")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Iterate over the DeleteRules and delete matching resources
	for _, rule := range sidecar.Spec.DeleteRules {
		namespaces := []string{}
		if rule.Namespace == "*" {
			// fetch all namespaces
			namespaceList := &corev1.NamespaceList{}
			if err := r.List(ctx, namespaceList); err != nil {
				logger.Error(err, "Failed to list namespaces")
				return ctrl.Result{}, err
			}
			for _, namespace := range namespaceList.Items {
				namespaces = append(namespaces, namespace.Name)
			}
		} else {
			namespaces = append(namespaces, rule.Namespace)
		}

		// determine the labels for the pods in the namespace
		var labelSelector labels.Selector
		if len(rule.Labels) == 0 || rule.Labels["*"] == "*" {
			labelSelector = labels.Everything()
		} else {
			// for _, labelSet := range rule.Labels {
			requirements := []labels.Requirement{}
			for key, value := range rule.Labels {
				requirement, err := labels.NewRequirement(key, selection.Equals, []string{value})
				if err != nil {
					logger.Error(err, "Failed to create label requirement")
					return ctrl.Result{}, err
				}
				requirements = append(requirements, *requirement)
			}
			labelSelector = labels.NewSelector().Add(requirements...)
			// }
		}

		// Delete all matching pods in the NS
		for _, namespace := range namespaces {
			podList := &corev1.PodList{}
			listOptions := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabelsSelector{Selector: labelSelector},
			}

			if err := r.List(ctx, podList, listOptions...); err != nil {
				logger.Error(err, "Failed to list pods")
				return ctrl.Result{}, err
			}

			for _, pod := range podList.Items {
				logger.Info("Deleting pod", "namespace", pod.Namespace, "name", pod.Name)
				if err := r.Delete(ctx, &pod); err != nil {
					logger.Error(err, "Failed to delete pod", "namespace", pod.Namespace, "name", pod.Name)
					return ctrl.Result{}, err
				}
			}
		}
	}

	sidecar.Status.Conditions = append(sidecar.Status.Conditions, metav1.Condition{
		Type:               "Reconciled",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		LastTransitionTime: metav1.Now(),
		Message:            fmt.Sprintf("Processed DeleteRules for %d namespaces", len(sidecar.Spec.DeleteRules)),
	})
	if err := r.Status().Update(ctx, sidecar); err != nil {
		logger.Error(err, "Failed to update Sidecar status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SidecarReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&terminatorv1alpha1.Sidecar{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
