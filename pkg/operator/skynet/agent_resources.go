/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Pattern follows submariner-operator reconcileGatewayDaemonSet / newGatewayDaemonSet:
build a desired workload object, then apply it with controller-runtime helpers.
SkyNet uses a Deployment (single-replica control plane) instead of a DaemonSet.
*/

package skynet

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
	"github.com/aswinsuryana/skynet/pkg/agent/bootstrap"
	"github.com/aswinsuryana/skynet/pkg/names"
	"github.com/aswinsuryana/skynet/pkg/operator/alloc"
)

const (
	// BrokerTokenSecretName is synced into the Skynet namespace so the agent pod can use secretKeyRef
	// (pods may only reference secrets in their own namespace).
	brokerTokenSecretName = "skynet-broker-api"
	brokerTokenSecretKey  = "token"
	brokerCASecretKey     = "ca.crt"
)

// newSkynetAgentDeployment returns the desired agent Deployment (cf. submariner newGatewayDaemonSet).
func newSkynetAgentDeployment(sk *skynetv1.Skynet, image string, pools *alloc.Pools) *appsv1.Deployment {
	replicas := int32(1)
	if sk.Spec.Agent != nil && sk.Spec.Agent.Replicas != nil && *sk.Spec.Agent.Replicas > 0 {
		replicas = *sk.Spec.Agent.Replicas
	}

	podLabels := map[string]string{
		names.AppLabel:       names.AppValue,
		names.ComponentLabel: names.AgentComponent,
		names.ManagedByLabel: names.ManagedByValue,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: sk.Namespace,
			Name:      names.AgentComponent,
			Labels:    podLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: newSkynetAgentPodTemplate(sk, image, podLabels, pools),
		},
	}
}

// newSkynetAgentPodTemplate builds the pod template with Submariner-style env wiring.
func newSkynetAgentPodTemplate(sk *skynetv1.Skynet, image string, podLabels map[string]string, pools *alloc.Pools) corev1.PodTemplateSpec {
	brokerNS := sk.Spec.BrokerConfig.BrokerNamespace
	if brokerNS == "" {
		brokerNS = bootstrap.DefaultBrokerNamespace
	}
	if pools == nil {
		pools = alloc.DefaultPools()
	}

	env := []corev1.EnvVar{
		{Name: bootstrap.EnvClusterID, Value: sk.Spec.ClusterID},
		{Name: bootstrap.EnvBrokerAPIServer, Value: sk.Spec.BrokerConfig.Server},
		{Name: bootstrap.EnvBrokerNamespace, Value: brokerNS},
		{
			Name: bootstrap.EnvBrokerToken,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: brokerTokenSecretName},
					Key:                  brokerTokenSecretKey,
				},
			},
		},
	}

	// Required by skynet-agent: values come from Skynet status (operator allocates or validates spec overrides).
	env = append(env,
		corev1.EnvVar{Name: bootstrap.EnvAllocatedVtepCIDR, Value: sk.Status.VtepCIDR},
		corev1.EnvVar{Name: bootstrap.EnvAllocatedASN, Value: strconv.FormatInt(int64(sk.Status.ASN), 10)},
		corev1.EnvVar{Name: bootstrap.EnvPoolASNMin, Value: strconv.FormatInt(int64(pools.ASNMin), 10)},
		corev1.EnvVar{Name: bootstrap.EnvPoolASNMax, Value: strconv.FormatInt(int64(pools.ASNMax), 10)},
		corev1.EnvVar{Name: bootstrap.EnvPoolVtepCIDR, Value: pools.VtepPoolCIDR},
		corev1.EnvVar{Name: bootstrap.EnvPoolVtepPrefixLen, Value: strconv.Itoa(pools.VtepPrefixLen)},
	)

	// Optional CA: mount if present in synced secret (empty key tolerated by optional pattern).
	env = append(env, corev1.EnvVar{
		Name: bootstrap.EnvBrokerCA,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: brokerTokenSecretName},
				Key:                  brokerCASecretKey,
				Optional:             ptr.To(true),
			},
		},
	})

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
		Spec: corev1.PodSpec{
			ServiceAccountName: names.AgentServiceAccount,
			Containers: []corev1.Container{
				{
					Name:  names.AgentComponent,
					Image: image,
					Env:   env,
				},
			},
		},
	}
}

// reconcileSkynetAgentDeployment ensures the agent Deployment exists and matches desired state.
// Returns the reconciled Deployment (cf. submariner-operator reconcileGatewayDaemonSet at
// https://github.com/submariner-io/submariner-operator/blob/devel/internal/controllers/submariner/submariner_controller.go#L172).
func reconcileSkynetAgentDeployment(
	ctx context.Context, c client.Client, scheme *runtime.Scheme, sk *skynetv1.Skynet, image string, pools *alloc.Pools, log logr.Logger,
) (*appsv1.Deployment, error) {
	desired := newSkynetAgentDeployment(sk, image, pools)
	key := types.NamespacedName{Namespace: sk.Namespace, Name: names.AgentComponent}

	dep := &appsv1.Deployment{}
	err := c.Get(ctx, key, dep)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "get skynet agent Deployment")
	}

	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(sk, desired, scheme); err != nil {
			return nil, errors.Wrap(err, "set controller reference on new Deployment")
		}
		log.Info("creating skynet-agent Deployment", "namespace", sk.Namespace)
		if err := c.Create(ctx, desired); err != nil {
			return nil, errors.Wrap(err, "create skynet-agent Deployment")
		}
		created := &appsv1.Deployment{}
		if err := c.Get(ctx, key, created); err != nil {
			return nil, errors.Wrap(err, "get skynet agent Deployment after create")
		}
		return created, nil
	}

	if err := controllerutil.SetControllerReference(sk, dep, scheme); err != nil {
		return nil, errors.Wrap(err, "set controller reference on Deployment")
	}

	dep.Labels = desired.Labels
	dep.Spec.Replicas = desired.Spec.Replicas
	dep.Spec.Selector = desired.Spec.Selector
	dep.Spec.Template = desired.Spec.Template

	log.V(1).Info("updating skynet-agent Deployment", "namespace", sk.Namespace)
	if err := c.Update(ctx, dep); err != nil {
		return nil, errors.Wrap(err, "update skynet-agent Deployment")
	}
	return dep, nil
}
