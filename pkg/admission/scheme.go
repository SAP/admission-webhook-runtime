/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import (
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var scheme = runtime.NewScheme()
var decoder runtime.Decoder

func init() {
	utilruntime.Must(admissionv1.AddToScheme(scheme))
	utilruntime.Must(admissionregistrationv1.AddToScheme(scheme))
	decoder = serializer.NewCodecFactory(scheme).UniversalDeserializer()
}
