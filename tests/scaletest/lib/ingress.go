/*
 * Copyright 2019-2020 VMware, Inc.
 * All Rights Reserved.
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*   http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

package lib

import (
	"context"
	"flag"
	"strconv"
	"testing"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

var ingressResource = schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "ingresses"}
var kubeClient dynamic.Interface
var coreV1Client corev1.CoreV1Interface
var appsV1Client appsv1.AppsV1Interface
var ctx = context.TODO()

const PORT = 8080
const SUBDOMAIN = ".avi.internal"
const SECRETNAME = "ingress-host-tls"
const INGRESSAPIVERSION = "extensions/v1beta1"
const UPDATEPATH = "new.host.internal"

func CreateApp(appName string, namespace string) error {
	deploymentSpec := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
		},
		Spec: appsV1.DeploymentSpec{
			ProgressDeadlineSeconds: func() *int32 { i := int32(600); return &i }(),
			RevisionHistoryLimit:    func() *int32 { i := int32(10); return &i }(),
			Replicas:                func() *int32 { i := int32(2); return &i }(),
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
				},
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: map[string]string{
						"app": appName,
					},
				},
				Spec: coreV1.PodSpec{
					Containers: []coreV1.Container{
						{
							Name:            appName,
							Image:           "avinetworks/server-os",
							ImagePullPolicy: func() coreV1.PullPolicy { str := coreV1.PullPolicy("IfNotPresent"); return str }(),
							Ports: []coreV1.ContainerPort{
								{
									Name:          "http",
									Protocol:      coreV1.ProtocolTCP,
									ContainerPort: PORT,
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: func() coreV1.TerminationMessagePolicy { str := coreV1.TerminationMessagePolicy("File"); return str }(),
						},
					},
					RestartPolicy:                 func() coreV1.RestartPolicy { str := coreV1.RestartPolicy("Always"); return str }(),
					TerminationGracePeriodSeconds: func() *int64 { i := int64(30); return &i }(),
				},
			},
		},
	}

	_, err := appsV1Client.Deployments(namespace).Create(ctx, deploymentSpec, metaV1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeleteApp(appName string, namespace string) error {
	err := appsV1Client.Deployments(namespace).Delete(ctx, appName, metaV1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func CreateService(serviceNamePrefix string, appName string, namespace string, num int) ([]string, error) {
	var listOfServicesCreated []string
	for i := 1; i <= num; i++ {
		serviceName := serviceNamePrefix + strconv.Itoa(i)
		serviceSpec := &coreV1.Service{
			ObjectMeta: metaV1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{
					"app": appName,
				},
				Ports: []coreV1.ServicePort{
					{
						Port: PORT,
						TargetPort: intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: PORT,
						},
					},
				},
			},
		}
		_, err := coreV1Client.Services(namespace).Create(ctx, serviceSpec, metaV1.CreateOptions{})
		if err != nil {
			return listOfServicesCreated, err
		}
		listOfServicesCreated = append(listOfServicesCreated, serviceName)
	}
	return listOfServicesCreated, nil
}

func DeleteService(serviceNameList []string, namespace string) error {
	for i := 0; i < len(serviceNameList); i++ {
		err := coreV1Client.Services(namespace).Delete(ctx, serviceNameList[i], metaV1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateInsecureIngress(ingressNamePrefix string, serviceName string, namespace string, num int, startIndex ...int) ([]string, []string, error) {
	var listOfIngressCreated []string
	var ingressHostNames []string
	var startInd int
	if len(startIndex) == 0 {
		startInd = 0
	} else {
		startInd = startIndex[0]
	}
	for i := startInd; i < num+startInd; i++ {
		ingressName := ingressNamePrefix + strconv.Itoa(i)
		hostName := ingressName + SUBDOMAIN
		ingress := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": INGRESSAPIVERSION,
				"kind":       "Ingress",
				"metadata": map[string]interface{}{
					"name":      ingressName,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"rules": []map[string]interface{}{
						{
							"host": hostName,
							"http": map[string]interface{}{
								"paths": []map[string]interface{}{
									{
										"backend": map[string]interface{}{
											"serviceName": serviceName,
											"servicePort": PORT,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_, err := kubeClient.Resource(ingressResource).Namespace(namespace).Create(ctx, ingress, metaV1.CreateOptions{})
		if err != nil {
			return nil, listOfIngressCreated, err
		}
		listOfIngressCreated = append(listOfIngressCreated, ingressName)
		ingressHostNames = append(ingressHostNames, hostName)

	}
	return listOfIngressCreated, ingressHostNames, nil
}

func CreateSecureIngress(ingressNamePrefix string, serviceName string, namespace string, num int, startIndex ...int) ([]string, []string, error) {
	var listOfIngressCreated []string
	var ingressHostNames []string
	var startInd int
	if len(startIndex) == 0 {
		startInd = 0
	} else {
		startInd = startIndex[0]
	}
	for i := startInd; i < num+startInd; i++ {
		ingressName := ingressNamePrefix + strconv.Itoa(i)
		hostName := ingressName + SUBDOMAIN
		ingress := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": INGRESSAPIVERSION,
				"kind":       "Ingress",
				"metadata": map[string]interface{}{
					"name":      ingressName,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"tls": []map[string]interface{}{
						{
							"secretName": SECRETNAME,
							"hosts": []interface{}{
								ingressName + SUBDOMAIN,
							},
						},
					},
					"rules": []map[string]interface{}{
						{
							"host": hostName,
							"http": map[string]interface{}{
								"paths": []map[string]interface{}{
									{
										"backend": map[string]interface{}{
											"serviceName": serviceName,
											"servicePort": PORT,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_, err := kubeClient.Resource(ingressResource).Namespace(namespace).Create(ctx, ingress, metaV1.CreateOptions{})
		if err != nil {
			return nil, listOfIngressCreated, err
		}
		listOfIngressCreated = append(listOfIngressCreated, ingressName)
		ingressHostNames = append(ingressHostNames, hostName)

	}
	return listOfIngressCreated, ingressHostNames, nil
}

func CreateMultiHostIngress(ingressNamePrefix string, listOfServices []string, namespace string, num int, startIndex ...int) ([]string, []string, []string, error) {
	var listOfIngressCreated []string
	var ingressSecureHostNames []string
	var ingressInsecureHostNames []string
	var startInd int
	if len(startIndex) == 0 {
		startInd = 0
	} else {
		startInd = startIndex[0]
	}
	for i := startInd; i < num+startInd; i++ {
		ingressName := ingressNamePrefix + "-multi-host-" + strconv.Itoa(i)
		hostNameSecure := ingressName + "-secure" + SUBDOMAIN
		hostNameInsecure := ingressName + "-insecure" + SUBDOMAIN
		ingress := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": INGRESSAPIVERSION,
				"kind":       "Ingress",
				"metadata": map[string]interface{}{
					"name":      ingressName,
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"tls": []map[string]interface{}{
						{
							"secretName": SECRETNAME,
							"hosts": []interface{}{
								hostNameSecure,
							},
						},
					},
					"rules": []map[string]interface{}{
						{
							"host": hostNameSecure,
							"http": map[string]interface{}{
								"paths": []map[string]interface{}{
									{
										"backend": map[string]interface{}{
											"serviceName": listOfServices[0],
											"servicePort": PORT,
										},
									},
								},
							},
						},
						{
							"host": hostNameInsecure,
							"http": map[string]interface{}{
								"paths": []map[string]interface{}{
									{
										"backend": map[string]interface{}{
											"serviceName": listOfServices[1],
											"servicePort": PORT,
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_, err := kubeClient.Resource(ingressResource).Namespace(namespace).Create(ctx, ingress, metaV1.CreateOptions{})
		if err != nil {
			return nil, nil, listOfIngressCreated, err
		}
		listOfIngressCreated = append(listOfIngressCreated, ingressName)
		ingressSecureHostNames = append(ingressSecureHostNames, hostNameSecure)
		ingressInsecureHostNames = append(ingressInsecureHostNames, hostNameInsecure)

	}
	return listOfIngressCreated, ingressSecureHostNames, ingressInsecureHostNames, nil
}

func DeleteIngress(namespace string, listOfIngressToDelete []string) ([]string, error) {
	var listOfDeletedIngresses []string
	for _, ing := range listOfIngressToDelete {
		ingressName := ing
		deletePolicy := metaV1.DeletePropagationForeground
		var zero int64 = 0
		deleteOptions := metaV1.DeleteOptions{
			GracePeriodSeconds: &zero,
			PropagationPolicy:  &deletePolicy,
		}
		if err := kubeClient.Resource(ingressResource).Namespace(namespace).Delete(ctx, ingressName, deleteOptions); err != nil {
			return listOfDeletedIngresses, err
		}
		listOfDeletedIngresses = append(listOfDeletedIngresses, ingressName)
	}
	return listOfDeletedIngresses, nil
}

func UpdateIngress(namespace string, listOfIngressToUpdate []string) ([]string, error) {
	for _, ing := range listOfIngressToUpdate {
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			result, getErr := kubeClient.Resource(ingressResource).Namespace(namespace).Get(ctx, ing, metaV1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			rules, found, err := unstructured.NestedSlice(result.Object, "spec", "rules")
			if err != nil || !found || rules == nil {
				return err
			}
			if err := unstructured.SetNestedField(rules[0].(map[string]interface{}), UPDATEPATH, "host"); err != nil {
				return err
			}
			if err := unstructured.SetNestedField(result.Object, rules, "spec", "rules"); err != nil {
				return err
			}
			_, updateErr := kubeClient.Resource(ingressResource).Namespace(namespace).Update(ctx, result, metaV1.UpdateOptions{})
			return updateErr
		})
		if retryErr != nil {
			return listOfIngressToUpdate, retryErr
		}
	}
	return listOfIngressToUpdate, nil
}

func ListIngress(t *testing.T, namespace string) ([]string, error) {
	var ingressList []string
	list, err := kubeClient.Resource(ingressResource).Namespace(namespace).List(ctx, metaV1.ListOptions{})
	if err != nil {
		return ingressList, err
	}
	for _, d := range list.Items {
		ingressList = append(ingressList, d.GetName())
	}
	return ingressList, nil
}

func DeletePod(podName string, namespace string) error {
	err := coreV1Client.Pods(namespace).Delete(ctx, podName, metaV1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func KubeInit(kubeconfig string) {
	kubeconfigFilePath := flag.String("kubeconfig", kubeconfig, "absolute path to the kubeconfig file")
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfigFilePath)
	if err != nil {
		panic(err)
	}
	kubeClient, err = dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	coreV1Client = clientset.CoreV1()
	appsV1Client = clientset.AppsV1()
}
