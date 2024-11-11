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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	terminatorv1alpha1 "github.com/cmwylie19/sidecar-terminator/api/v1alpha1"
)

var _ = Describe("Sidecar Controller", func() {
	Context("When reconciling a Sidecar resource with IgnoreRules", func() {
		const resourceName = "test-resource"
		const targetPodLabel = "delete-me"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		sidecar := &terminatorv1alpha1.Sidecar{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Sidecar")
			err := k8sClient.Get(ctx, typeNamespacedName, sidecar)
			if err != nil && errors.IsNotFound(err) {
				resource := &terminatorv1alpha1.Sidecar{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: terminatorv1alpha1.SidecarSpec{
						DeleteRules: []terminatorv1alpha1.DeleteRules{
							{
								Namespace: "default",
								Labels: []map[string]string{
									{
										"app": targetPodLabel,
									}},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating a target pod that should be deleted")
			targetPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						"app": targetPodLabel,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "nginx",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, targetPod)).To(Succeed())
		})
		AfterEach(func() {
			By("Cleanup the Sidecar resource and target pods")
			Expect(k8sClient.Delete(ctx, sidecar)).To(Succeed())

			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList, &client.ListOptions{
				Namespace:     "default",
				LabelSelector: labels.SelectorFromSet(map[string]string{"app": targetPodLabel}),
			})).To(Succeed())

			for _, pod := range podList.Items {
				Expect(k8sClient.Delete(ctx, &pod)).To(Succeed())
			}

			resource := &terminatorv1alpha1.Sidecar{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Sidecar")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SidecarReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the target pod has been deleted")
			deletedPod := &corev1.Pod{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-pod",
				Namespace: "default",
			}, deletedPod)
			Expect(err).To(HaveOccurred())

			By("vieryfing the sidecar status condition is set to reconciled")
			Expect(k8sClient.Get(ctx, typeNamespacedName, sidecar)).To(Succeed())
			Expect(sidecar.Status.Conditions).NotTo(BeEmpty())
			Expect(sidecar.Status.Conditions[0].Type).To(Equal("Reconciled"))
			Expect(sidecar.Status.Conditions[0].Status).To(Equal(corev1.ConditionTrue))
		})
	})
})
