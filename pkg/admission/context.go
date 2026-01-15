/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and admission-webhook-runtime contributors
SPDX-License-Identifier: Apache-2.0
*/

package admission

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
)

type (
	admissionRequestContextKeyType struct{}
)

var (
	admissionRequestContextKey = admissionRequestContextKeyType{}
)

func contextWithAdmissionRequest(ctx context.Context, req *admissionv1.AdmissionRequest) context.Context {
	return context.WithValue(ctx, admissionRequestContextKey, req)
}

func AdmissionRequestFromContext(ctx context.Context) (*admissionv1.AdmissionRequest, error) {
	if admissionRequest, ok := ctx.Value(admissionRequestContextKey).(*admissionv1.AdmissionRequest); ok {
		return admissionRequest, nil
	}
	return nil, fmt.Errorf("admission request not found in context")
}
