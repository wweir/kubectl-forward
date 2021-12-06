package main

import (
	"fmt"

	"github.com/sower-proxy/deferlog/log"
	corev1 "k8s.io/api/core/v1"
	listercorev1 "k8s.io/client-go/listers/core/v1"
)

type SvcTarget struct {
	DefaultNS      string
	SvcLister      listercorev1.ServiceLister
	EndpointLister listercorev1.EndpointsLister

	TargetM map[string]*forwardTarget
}

func (tgt *SvcTarget) OnAdd(obj interface{}) {
	switch kubeObj := obj.(type) {
	case *corev1.Service:
		endpoint, err := tgt.EndpointLister.Endpoints(kubeObj.Namespace).Get(kubeObj.Name)
		if err != nil {
			log.Warn().Err(err).Msg("get endpoint")
			return
		}
		tgt.addAddressByService(kubeObj, endpoint)

	case *corev1.Endpoints:
		svc, err := tgt.SvcLister.Services(kubeObj.Namespace).Get(kubeObj.Name)
		if err != nil {
			// log.Warn().Err(err).Msg("get service")
			return
		}
		tgt.addAddressByService(svc, kubeObj)

	default:
		log.Warn().
			Interface("obj", obj).
			Msg("unknown type")
	}
}
func (tgt *SvcTarget) OnUpdate(oldObj, newObj interface{}) { tgt.OnAdd(newObj) }
func (svc *SvcTarget) OnDelete(obj interface{})            {}

const serviceNSFormat = "%s:%d"

var serviceFormats = [...]string{
	"%s.%s:%d",
	"%s.%s.svc:%d",
	"%s.%s.svc.cluster.local:%d",
}

func (tgt *SvcTarget) addAddressByService(svc *corev1.Service, endpoint *corev1.Endpoints) {

	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		tgt.addAddrsByEndpoint(endpoint)
		return
	}

	isDefaultNS := svc.Namespace == tgt.DefaultNS

	pods := make([]string, 0, len(endpoint.Subsets))
	for _, s := range endpoint.Subsets {
		for _, addr := range s.Addresses {
			if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
				continue
			}

			pods = append(pods, addr.TargetRef.Name)
		}
	}

	for _, port := range svc.Spec.Ports {
		target := forwardTarget{
			Namespace: svc.Namespace,
			Pods:      pods,
			Port:      port.Port,
		}

		if isDefaultNS {
			tgt.TargetM[fmt.Sprintf(serviceNSFormat, svc.Name, port.Port)] = &target
		}
		for _, nsFormat := range serviceFormats {
			tgt.TargetM[fmt.Sprintf(nsFormat, svc.Name, svc.Namespace, port.Port)] = &target
		}
	}
	return
}

const endpointNSFormat = "%s.%s:%d"

var endpointFormats = [...]string{
	"%s.%s.%s:%d",
	"%s.%s.%s.svc:%d",
	"%s.%s.%s.svc.cluster.local:%d",
}

func (tgt *SvcTarget) addAddrsByEndpoint(endpoint *corev1.Endpoints) {
	isDefaultNS := endpoint.Namespace == tgt.DefaultNS

	for s := range endpoint.Subsets {
		subset := &endpoint.Subsets[s]
		for p := range subset.Ports {

			for _, addr := range subset.Addresses {
				if addr.TargetRef == nil || addr.TargetRef.Kind != "Pod" {
					continue
				}

				target := forwardTarget{
					Namespace: endpoint.Namespace,
					Port:      subset.Ports[p].Port,
				}

				target.Pods = append(target.Pods, addr.TargetRef.Name)

				if isDefaultNS {
					tgt.TargetM[fmt.Sprintf(endpointNSFormat,
						addr.Hostname, endpoint.Name, subset.Ports[p].Port)] = &target
				}
				for _, nsFormat := range endpointFormats {
					tgt.TargetM[fmt.Sprintf(nsFormat,
						addr.Hostname, endpoint.Name, endpoint.Namespace, subset.Ports[p].Port)] = &target
				}
			}
		}
	}
}
