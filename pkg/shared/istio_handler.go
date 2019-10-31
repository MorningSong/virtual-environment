package shared

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis/istio/common/v1alpha1"
	networkingv1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// generate istio virtual service instance
func VirtualService(name string, namespace string, availableLabels []string, relatedDeployments map[string]string,
	veHeader string, veSplitter string) *networkingv1alpha3.VirtualService {
	virtualSrv := &networkingv1alpha3.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha3.VirtualServiceSpec{
			Hosts: []string{name},
			HTTP: []networkingv1alpha3.HTTPRoute{{
				Route: []networkingv1alpha3.HTTPRouteDestination{{
					Destination: networkingv1alpha3.Destination{
						Host: name,
					},
				}},
			}},
		},
	}
	for _, label := range availableLabels {
		matchRoute, ok := virtualServiceMatchRoute(name, relatedDeployments, label, veHeader, veSplitter)
		if ok {
			virtualSrv.Spec.HTTP = append(virtualSrv.Spec.HTTP, matchRoute)
		}
	}
	return virtualSrv
}

// generate istio destination rule instance
func DestinationRule(name string, namespace string, relatedDeployments map[string]string,
	veLabel string) *networkingv1alpha3.DestinationRule {
	destRule := &networkingv1alpha3.DestinationRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1alpha3.DestinationRuleSpec{
			Host:    name,
			Subsets: []networkingv1alpha3.Subset{},
		},
	}
	for dep, label := range relatedDeployments {
		destRule.Spec.Subsets = append(destRule.Spec.Subsets, destinationRuleMatchSubset(dep, veLabel, label))
	}
	return destRule
}

// check whether DestinationRule is different
func IsDifferentDestinationRule(spec1 networkingv1alpha3.DestinationRuleSpec,
	spec2 networkingv1alpha3.DestinationRuleSpec, label string) bool {
	if len(spec1.Subsets) != len(spec2.Subsets) {
		return true
	}
	for _, subset1 := range spec1.Subsets {
		subset2 := findSubsetByName(spec2.Subsets, subset1.Name)
		if subset2 == nil {
			return true
		}
		if subset1.Labels[label] != subset2.Labels[label] {
			return true
		}
	}
	return false
}

// check whether VirtualService is different
func IsDifferentVirtualService(spec1 networkingv1alpha3.VirtualServiceSpec, spec2 networkingv1alpha3.VirtualServiceSpec, header string) bool {
	if len(spec1.HTTP) != len(spec2.HTTP) {
		return true
	}
	for _, route1 := range spec1.HTTP {
		if route1.Match == nil {
			continue
		}
		if !findMatchRoute(spec2.HTTP, &route1, header) {
			return true
		}
	}
	return false
}

// return map of deployment name to virtual label value
func FindAllRelatedDeployments(deployments map[string]map[string]string, selector map[string]string, velabel string) map[string]string {
	relatedDeployments := make(map[string]string)
	for dep, labels := range deployments {
		match := true
		for k, v := range selector {
			if labels[k] != v {
				match = false
				break
			}
		}
		if _, exist := labels[velabel]; match && exist {
			relatedDeployments[dep] = labels[velabel]
		}
	}
	return relatedDeployments
}

// list all possible values in deployment virtual env label
func FindAllVirtualEnvLabelValues(deployments map[string]map[string]string, velabel string) []string {
	labelSet := make(map[string]bool)
	for _, labels := range deployments {
		labelVal, exist := labels[velabel]
		if exist {
			labelSet[labelVal] = true
		}
	}
	return getKeys(labelSet)
}

// delete specified instance
func DeleteIns(client client.Client, namespace string, name string, obj runtime.Object) error {
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, obj)
	if err != nil {
		return err
	}
	err = client.Delete(context.TODO(), obj)
	if err != nil {
		return err
	}
	return nil
}

// find subset from list
func findSubsetByName(subsets []networkingv1alpha3.Subset, name string) *networkingv1alpha3.Subset {
	for _, subset := range subsets {
		if subset.Name == name {
			return &subset
		}
	}
	return nil
}

// check whether HTTPRoute exist in list
func findMatchRoute(routes []networkingv1alpha3.HTTPRoute, target *networkingv1alpha3.HTTPRoute, header string) bool {
	for _, route := range routes {
		if route.Match == nil {
			continue
		}
		if route.Route[0].Destination.Subset == target.Route[0].Destination.Subset &&
			route.Match[0].Headers[header] == target.Match[0].Headers[header] {
			return true
		}
	}
	return false
}

// generate istio destination rule subset instance
func destinationRuleMatchSubset(name string, labelKey string, labelValue string) networkingv1alpha3.Subset {
	return networkingv1alpha3.Subset{
		Name: name,
		Labels: map[string]string{
			labelKey: labelValue,
		},
	}
}

// get all keys of a map as array
func getKeys(kv map[string]bool) []string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	return keys
}

// calculate and generate http route instance
func virtualServiceMatchRoute(serviceName string, relatedDeployments map[string]string, labelVal string, headerKey string,
	splitter string) (networkingv1alpha3.HTTPRoute, bool) {
	var possibleRoutes []string
	for k, v := range relatedDeployments {
		if leveledEqual(v, labelVal, splitter) {
			possibleRoutes = append(possibleRoutes, k)
		}
	}
	if len(possibleRoutes) > 0 {
		return matchRoute(serviceName, headerKey, labelVal, findLongestString(possibleRoutes)), true
	}
	return networkingv1alpha3.HTTPRoute{}, false
}

// generate istio virtual service http route instance
func matchRoute(serviceName string, headerKey string, labelVal string, matchedLabel string) networkingv1alpha3.HTTPRoute {
	return networkingv1alpha3.HTTPRoute{
		Route: []networkingv1alpha3.HTTPRouteDestination{{
			Destination: networkingv1alpha3.Destination{
				Host:   serviceName,
				Subset: matchedLabel,
			},
		}},
		Match: []networkingv1alpha3.HTTPMatchRequest{{
			Headers: map[string]v1alpha1.StringMatch{
				headerKey: {
					Exact: labelVal,
				},
			},
		}},
	}
}

// get the longest string in list
func findLongestString(strings []string) string {
	mostLongStr := ""
	for _, str := range strings {
		if len(str) > len(mostLongStr) {
			mostLongStr = str
		}
	}
	return mostLongStr
}

// check whether source string match target string at any level
func leveledEqual(source string, target string, splitter string) bool {
	for {
		if source == target {
			return true
		}
		if strings.Contains(source, splitter) {
			source = source[0:strings.LastIndex(source, splitter)]
		} else {
			return false
		}
	}
}