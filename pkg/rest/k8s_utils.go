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

package rest

import (
	"errors"
	"strings"

	core "k8s.io/api/core/v1"

	avicache "ako/pkg/cache"
	"ako/pkg/lib"

	"github.com/avinetworks/container-lib/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func UpdateIngressStatus(vs_cache_obj *avicache.AviVsCache, svc_mdata_obj avicache.ServiceMetadataObj, key string) error {
	var err error
	if len(svc_mdata_obj.NamespaceIngressName) > 0 {
		// This is SNI with hostname sharding.
		for _, ingressns := range svc_mdata_obj.NamespaceIngressName {
			ingressArr := strings.Split(ingressns, "/")
			if len(ingressArr) != 2 {
				return errors.New("key: %s, msg: UpdateIngressStatus IngressNamespace format not correct")
			}
			err = updateObject(ingressArr[0], ingressArr[1], svc_mdata_obj.HostNames, vs_cache_obj, key)
		}
	} else {
		err = updateObject(svc_mdata_obj.Namespace, svc_mdata_obj.IngressName, svc_mdata_obj.HostNames, vs_cache_obj, key)
	}

	if err != nil {
		utils.AviLog.Error(err)
	}
	return err
}

func updateObject(namespace, ingressname string, hostnames []string, vs_cache_obj *avicache.AviVsCache, key string, retryNum ...int) error {
	retry := 0
	if len(retryNum) > 0 {
		retry = retryNum[0]
		if retry >= 2 {
			return errors.New("key: %s, msg: UpdateIngressStatus retried 3 times, aborting")
		}
	}
	var ingObj interface{}
	var err error
	mClient := utils.GetInformers().ClientSet
	if lib.GetIngressApi() == utils.ExtV1IngressInformer {
		ingObj, err = mClient.ExtensionsV1beta1().Ingresses(namespace).Get(ingressname, metav1.GetOptions{})
	} else {
		ingObj, err = mClient.NetworkingV1beta1().Ingresses(namespace).Get(ingressname, metav1.GetOptions{})
	}
	if err != nil {
		utils.AviLog.Warnf("key: %s, msg: Could not get the ingress object for UpdateStatus :%s", key, err)
		return err
	}

	mIngress, ok := utils.ToNetworkingIngress(ingObj)
	if !ok {
		utils.AviLog.Errorf("Unable to convert obj type interface to networking/v1beta1 ingress")
	}

	// Clean up all hosts that are not part of the ingress spec.
	var hostListIng []string
	for _, rule := range mIngress.Spec.Rules {
		hostListIng = append(hostListIng, rule.Host)
	}

	noOP := false
	for _, status := range mIngress.Status.LoadBalancer.Ingress {
		if status.IP == vs_cache_obj.Vip && utils.HasElem(hostnames, status.Hostname) {
			utils.AviLog.Debugf("key: %s, msg: No changes detected in ingress status. Hostnames: %+v VsCache: %+v", hostnames, utils.Stringify(vs_cache_obj))
			noOP = true
		}
	}

	// If we find a hostname in the present update, let's first remove it from the existing status.
	utils.AviLog.Debugf("key: %s, msg: status before update: %v", key, mIngress.Status.LoadBalancer.Ingress)
	for i := len(mIngress.Status.LoadBalancer.Ingress) - 1; i >= 0; i-- {
		if utils.HasElem(hostnames, mIngress.Status.LoadBalancer.Ingress[i].Hostname) && !noOP {
			mIngress.Status.LoadBalancer.Ingress = append(mIngress.Status.LoadBalancer.Ingress[:i], mIngress.Status.LoadBalancer.Ingress[i+1:]...)
		}
	}

	// Handle fresh hostname update
	if !noOP {
		for _, host := range hostnames {
			lbIngress := core.LoadBalancerIngress{
				IP:       vs_cache_obj.Vip,
				Hostname: host,
			}
			mIngress.Status.LoadBalancer.Ingress = append(mIngress.Status.LoadBalancer.Ingress, lbIngress)
		}
	}

	// remove the host from status which is not in spec
	for i := len(mIngress.Status.LoadBalancer.Ingress) - 1; i >= 0; i-- {
		if !utils.HasElem(hostListIng, mIngress.Status.LoadBalancer.Ingress[i].Hostname) {
			mIngress.Status.LoadBalancer.Ingress = append(mIngress.Status.LoadBalancer.Ingress[:i], mIngress.Status.LoadBalancer.Ingress[i+1:]...)
		}
	}
	utils.AviLog.Infof("key: %s, msg: status after update: %v", key, mIngress.Status.LoadBalancer.Ingress)

	if lib.GetIngressApi() == utils.ExtV1IngressInformer {
		mIng, ok := utils.ToExtensionIngress(mIngress)
		if !ok {
			utils.AviLog.Errorf("Unable to convert obj type interface to extensions/v1beta1 ingress")
		}

		_, err = mClient.ExtensionsV1beta1().Ingresses(namespace).UpdateStatus(mIng)
	} else {
		_, err = mClient.NetworkingV1beta1().Ingresses(namespace).UpdateStatus(mIngress)
	}
	if err != nil {
		utils.AviLog.Errorf("key: %s, msg: there was an error in updating the ingress status: %v", key, err)
		return updateObject(namespace, ingressname, hostnames, vs_cache_obj, key, retry+1)
	}
	utils.AviLog.Infof("key:%s, msg: Successfully updated the ingress status of ingress: %s ns: %s", key, ingressname, namespace)
	return err
}

func DeleteIngressStatus(svc_mdata_obj avicache.ServiceMetadataObj, isVSDelete bool, key string) error {
	var err error
	if len(svc_mdata_obj.NamespaceIngressName) > 0 {
		// This is SNI with hostname sharding.
		for _, ingressns := range svc_mdata_obj.NamespaceIngressName {
			ingressArr := strings.Split(ingressns, "/")
			if len(ingressArr) != 2 {
				return errors.New("key: %s, msg: DeleteIngressStatus IngressNamespace format not correct")
			}
			svc_mdata_obj.Namespace = ingressArr[0]
			svc_mdata_obj.IngressName = ingressArr[1]
			err = deleteObject(svc_mdata_obj, key, isVSDelete)
		}
	} else {
		err = deleteObject(svc_mdata_obj, key, isVSDelete)
	}

	if err != nil {
		utils.AviLog.Error(err)
	}
	return err
}

func deleteObject(svc_mdata_obj avicache.ServiceMetadataObj, key string, isVSDelete bool, retryNum ...int) error {
	retry := 0
	if len(retryNum) > 0 {
		utils.AviLog.Infof("key:%s, msg: Retrying to update the ingress status", key)
		retry = retryNum[0]
		if retry >= 2 {
			return errors.New("key: %s, msg: DeleteIngressStatus retried 3 times, aborting")
		}
	}

	mClient := utils.GetInformers().ClientSet
	var ingObj interface{}
	var err error

	if lib.GetIngressApi() == utils.ExtV1IngressInformer {
		ingObj, err = mClient.ExtensionsV1beta1().Ingresses(svc_mdata_obj.Namespace).Get(svc_mdata_obj.IngressName, metav1.GetOptions{})
	} else {
		ingObj, err = mClient.NetworkingV1beta1().Ingresses(svc_mdata_obj.Namespace).Get(svc_mdata_obj.IngressName, metav1.GetOptions{})
	}

	if err != nil {
		utils.AviLog.Warnf("key: %s, msg: Could not get the ingress object for DeleteStatus :%s", key, err)
		return err
	}

	mIngress, ok := utils.ToNetworkingIngress(ingObj)
	if !ok {
		utils.AviLog.Errorf("Unable to convert obj type interface to networking/v1beta1 ingress")
	}
	var hostListIng []string
	for _, rule := range mIngress.Spec.Rules {
		hostListIng = append(hostListIng, rule.Host)
	}
	utils.AviLog.Infof("key: %s, msg: status before update: %v", key, mIngress.Status.LoadBalancer.Ingress)

	for i, status := range mIngress.Status.LoadBalancer.Ingress {
		for _, host := range svc_mdata_obj.HostNames {
			if status.Hostname == host {
				// Check if this host is still present in the spec, if so - don't delete it
				if !utils.HasElem(hostListIng, host) || isVSDelete {
					mIngress.Status.LoadBalancer.Ingress = append(mIngress.Status.LoadBalancer.Ingress[:i], mIngress.Status.LoadBalancer.Ingress[i+1:]...)
				} else {
					utils.AviLog.Debugf("key: %s, msg: skipping status update since host is present in the ingress: %v", key, host)
				}
			}
		}
	}
	utils.AviLog.Infof("key: %s, msg: status after update: %v", key, mIngress.Status.LoadBalancer.Ingress)

	var response interface{}
	if lib.GetIngressApi() == utils.ExtV1IngressInformer {
		mIng, ok := utils.ToExtensionIngress(mIngress)
		if !ok {
			utils.AviLog.Errorf("Unable to convert obj type interface to extensions/v1beta1 ingress")
		}

		response, err = mClient.ExtensionsV1beta1().Ingresses(mIngress.Namespace).UpdateStatus(mIng)
	} else {
		response, err = mClient.NetworkingV1beta1().Ingresses(svc_mdata_obj.Namespace).UpdateStatus(mIngress)
	}
	if err != nil {
		utils.AviLog.Errorf("key: %s, msg: there was an error in deleting the ingress status: %v", key, err)
		return deleteObject(svc_mdata_obj, key, isVSDelete, retry+1)
	}

	utils.AviLog.Infof("key:%s, msg: Successfully deleted the ingress status: %v", key, utils.Stringify(response))
	return nil
}

func UpdateL4LBStatus(vs_cache_obj *avicache.AviVsCache, svc_mdata_obj avicache.ServiceMetadataObj, key string) error {
	mClient := utils.GetInformers().ClientSet
	mLb, err := mClient.CoreV1().Services(svc_mdata_obj.Namespace).Get(svc_mdata_obj.ServiceName, metav1.GetOptions{})
	if err != nil {
		utils.AviLog.Warnf("key: %s, msg: there was a problem in updating the service status :%s", key, err)
		return err
	}
	if len(svc_mdata_obj.HostNames) != 1 {
		return errors.New("Hostname length not appropriate for status update, not equals 1")
	}
	// Once the vsvip object is available - we should be able to update the hostname, for now just updating the vip
	lbIngress := core.LoadBalancerIngress{
		IP:       vs_cache_obj.Vip,
		Hostname: svc_mdata_obj.HostNames[0],
	}
	mLb.Status = core.ServiceStatus{
		LoadBalancer: core.LoadBalancerStatus{
			Ingress: []core.LoadBalancerIngress{lbIngress},
		},
	}
	response, err := mClient.CoreV1().Services(svc_mdata_obj.Namespace).UpdateStatus(mLb)
	if err != nil {
		utils.AviLog.Errorf("key: %s, msg: there was an error in updating the loadbalancer status: %v", key, err)
		return err
	}
	utils.AviLog.Infof("key:%s, msg: Successfully updated the loadbalancer status: %v", key, utils.Stringify(response))
	return nil
}

func DeleteL4LBStatus(svc_mdata_obj avicache.ServiceMetadataObj, key string) error {
	mClient := utils.GetInformers().ClientSet
	mLb, err := mClient.CoreV1().Services(svc_mdata_obj.Namespace).Get(svc_mdata_obj.ServiceName, metav1.GetOptions{})
	if err != nil {
		utils.AviLog.Warnf("key: %s, msg: there was a problem in resetting the service status :%s", key, err)
		return err
	}
	mLb.Status = core.ServiceStatus{
		LoadBalancer: core.LoadBalancerStatus{
			Ingress: []core.LoadBalancerIngress{},
		},
	}
	response, err := mClient.CoreV1().Services(svc_mdata_obj.Namespace).UpdateStatus(mLb)
	if err != nil {
		utils.AviLog.Errorf("key: %s, msg: there was an error in resetting the loadbalancer status: %v", key, err)
		return err
	}
	utils.AviLog.Infof("key:%s, msg: Successfully reset the loadbalancer status: %v", key, utils.Stringify(response))
	return nil
}

// SyncIngressStatus gets data from L3 cache and does a status update on the ingress objects
// based on the service metadata objects it finds in the cache
// This is executed once AKO is done with populating the L3 cache in reboot scenarios
func (rest *RestOperations) SyncIngressStatus() {
	vsKeys := rest.cache.VsCacheMeta.AviGetAllVSKeys()
	utils.AviLog.Debugf("Ingress status sync for vsKeys %+v", utils.Stringify(vsKeys))

	for _, vsKey := range vsKeys {
		vsCache, ok := rest.cache.VsCacheMeta.AviCacheGet(vsKey)
		if !ok {
			continue
		}

		vsCacheObj, found := vsCache.(*avicache.AviVsCache)
		if !found {
			continue
		}

		parentVsKey := vsCacheObj.ParentVSRef
		if parentVsKey != (avicache.NamespaceName{}) {
			// secure VSes handler
			parentVs, found := rest.cache.VsCacheMeta.AviCacheGet(parentVsKey)
			if !found {
				continue
			}

			vsSvcMetadataObj := vsCacheObj.ServiceMetadataObj
			parentVsObj, _ := parentVs.(*avicache.AviVsCache)
			if (vsSvcMetadataObj.IngressName != "" || len(vsSvcMetadataObj.NamespaceIngressName) > 0) && vsSvcMetadataObj.Namespace != "" && parentVsObj != nil {
				// sni VS
				UpdateIngressStatus(parentVsObj, vsSvcMetadataObj, "syncstatus")
			}
		} else {
			// insecure VSes handler
			for _, poolKey := range vsCacheObj.PoolKeyCollection {
				poolCache, ok := rest.cache.PoolCache.AviCacheGet(poolKey)
				if !ok {
					continue
				}

				poolCacheObj, found := poolCache.(*avicache.AviPoolCache)
				if !found {
					continue
				}

				// insecure pools
				if poolCacheObj.ServiceMetadataObj.Namespace != "" {
					UpdateIngressStatus(vsCacheObj, poolCacheObj.ServiceMetadataObj, "syncstatus")
				}
			}
		}
	}

	return
}
