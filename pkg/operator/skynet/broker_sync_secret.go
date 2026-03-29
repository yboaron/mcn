/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the SkyNet project.

Agent pods may only reference Secrets in their own namespace. This syncs broker credentials from
spec.brokerConfig.tokenSecretRef into the Skynet namespace as brokerTokenSecretName.
*/

package skynet

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	skynetv1 "github.com/aswinsuryana/skynet/pkg/apis/skynet.io/v1"
)

func reconcileBrokerTokenSecret(ctx context.Context, c client.Client, scheme *runtime.Scheme, sk *skynetv1.Skynet) error {
	ref := sk.Spec.BrokerConfig.TokenSecretRef
	key := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}

	var src corev1.Secret
	if err := c.Get(ctx, key, &src); err != nil {
		return errors.Wrapf(err, "read broker token secret %s/%s", ref.Namespace, ref.Name)
	}

	token := src.Data[corev1.ServiceAccountTokenKey]
	if len(token) == 0 {
		token = src.Data["token"]
	}
	if len(token) == 0 {
		return errors.Errorf("broker secret %s/%s has no token key (expected %q or \"token\")",
			ref.Namespace, ref.Name, corev1.ServiceAccountTokenKey)
	}

	dst := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: sk.Namespace,
			Name:      brokerTokenSecretName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, dst, func() error {
		if err := controllerutil.SetControllerReference(sk, dst, scheme); err != nil {
			return errors.Wrap(err, "set controller ref on broker api secret")
		}
		if dst.Data == nil {
			dst.Data = map[string][]byte{}
		}
		dst.Data[brokerTokenSecretKey] = token
		if ca := src.Data["ca.crt"]; len(ca) > 0 {
			dst.Data[brokerCASecretKey] = ca
		} else {
			delete(dst.Data, brokerCASecretKey)
		}
		dst.Type = corev1.SecretTypeOpaque
		return nil
	})

	return errors.Wrap(err, "sync broker token secret into Skynet namespace")
}
