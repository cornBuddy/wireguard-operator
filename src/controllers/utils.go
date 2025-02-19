package controllers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/ahova/ahova-vpn/services/wireguard-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cisco-open/k8s-objectmatcher/patch"
)

const (
	wireguardRef = ".spec.wireguardRef"
)

type endpointExtractor func(v1alpha1.Wireguard, corev1.Service) string

func extractClusterIp(_ v1alpha1.Wireguard, svc corev1.Service) string {
	return svc.Spec.ClusterIP
}

func extractWireguardEndpoint(wg v1alpha1.Wireguard, _ corev1.Service) string {
	return *wg.Spec.EndpointAddress
}

type reconciler interface {
	client.Reader
	client.Writer
}

// creates or updates resource. returns true when created or updated
func apply(ctx context.Context, r reconciler, desired client.Object) (
	bool, error) {

	var current client.Object
	switch v := desired.(type) {
	case *corev1.Service:
		current = &corev1.Service{}
	case *corev1.ConfigMap:
		current = &corev1.ConfigMap{}
	case *corev1.Secret:
		current = &corev1.Secret{}
	case *appsv1.Deployment:
		current = &appsv1.Deployment{}
	case nil:
		return false, fmt.Errorf("desired cannot be nil")
	default:
		return false, fmt.Errorf("unsupported type %s for desired", v)
	}

	key := types.NamespacedName{
		Name:      desired.GetName(),
		Namespace: desired.GetNamespace(),
	}
	err := r.Get(ctx, key, current)
	objectNotExists := errors.IsNotFound(err)
	unexpectedError := err != nil && !objectNotExists

	if unexpectedError {
		return false, err
	}

	if objectNotExists {
		if err := r.Create(ctx, desired); err != nil {
			return false, err
		}

		// resource is created, get back to reconcilation
		return true, nil
	}

	patchMaker := patch.DefaultPatchMaker
	opts := []patch.CalculateOption{
		patch.IgnoreField("metadata"),
	}
	patchResult, err := patchMaker.Calculate(current, desired, opts...)
	if err != nil {
		return false, err
	}

	// nothing to update, get back to reconcilation
	if patchResult.IsEmpty() {
		return false, nil
	}

	if err := r.Update(ctx, desired); err != nil {
		return false, err
	}

	// resource is updated, get back to reconcilation
	return true, nil
}

func makeHash(data []byte) string {
	hash := sha1.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

func toPtr[V any](o V) *V { return &o }
